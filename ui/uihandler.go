// Copyright 2017 Istio Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package ui // import "fortio.org/fortio/ui"

import (
	"bytes"
	"context"

	// nolint: gosec // md5 is mandated, not our choice
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"html"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"fortio.org/fortio/dflag/endpoint"
	"fortio.org/fortio/fgrpc"
	"fortio.org/fortio/fhttp"
	"fortio.org/fortio/fnet"
	"fortio.org/fortio/log"
	"fortio.org/fortio/periodic"
	"fortio.org/fortio/stats"
	"fortio.org/fortio/tcprunner"
	"fortio.org/fortio/udprunner"
	"fortio.org/fortio/version"
)

// TODO: move some of those in their own files/package (e.g data transfer TSV)
// and add unit tests.

var (
	// UI and Debug prefix/paths (read in ui handler).
	uiPath      string // absolute (base)
	logoPath    string // relative
	chartJSPath string // relative
	debugPath   string // mostly relative
	fetchPath   string // this one is absolute
	// Used to construct default URL to self.
	urlHostPort string
	// Start time of the UI Server (for uptime info).
	startTime time.Time
	// Directory where the static content and templates are to be loaded from.
	// This is replaced at link time to the packaged directory (e.g /usr/share/fortio/)
	// but when fortio is installed with go get we use RunTime to find that directory.
	// (see Dockerfile for how to set it).
	resourcesDir     string
	extraBrowseLabel string // Extra label for report only
	// Directory where results are written to/read from.
	dataDir        string
	mainTemplate   *template.Template
	browseTemplate *template.Template
	syncTemplate   *template.Template
	uiRunMapMutex  = &sync.Mutex{}
	id             int64
	runs           = make(map[int64]*periodic.RunnerOptions)
	// Base URL used for index - useful when running under an ingress with prefix.
	baseURL string

	defaultPercentileList []float64
)

const (
	fetchURI    = "fetch/"
	fetch2URI   = "fetch2/"
	faviconPath = "/favicon.ico"
	modegrpc    = "grpc"
)

// Gets the resources directory from one of 3 sources.
func getResourcesDir(override string) string {
	if override != "" {
		log.Infof("Using resources directory from override: %s", override)
		return override
	}
	if resourcesDir != "" {
		log.LogVf("Using resources directory set at link time: %s", resourcesDir)
		return resourcesDir
	}
	_, filename, _, ok := runtime.Caller(0)
	log.LogVf("Guessing resources directory from runtime source location: %v - %s", ok, filename)
	if ok {
		return path.Dir(filename)
	}
	log.Errf("Unable to get source tree location. Failing to serve static contents.")
	return ""
}

// TODO: auto map from (Http)RunnerOptions to form generation and/or accept
// JSON serialized options as input.

// TODO: unit tests, allow additional data sets.

type mode int

// The main html has 3 principal modes.
const (
	// Default: renders the forms/menus.
	menu mode = iota
	// Trigger a run.
	run
	// Request abort.
	stop
)

// Handler is the main UI handler creating the web forms and processing them.
// nolint: funlen, gocognit, gocyclo, nestif // should be refactored indeed (TODO)
func Handler(w http.ResponseWriter, r *http.Request) {
	fhttp.LogRequest(r, "UI")
	mode := menu
	JSONOnly := false
	DoSave := (r.FormValue("save") == "on")
	url := r.FormValue("url")
	runid := int64(0)
	runner := r.FormValue("runner")
	if r.FormValue("load") == "Start" {
		mode = run
		if r.FormValue("json") == "on" {
			JSONOnly = true
			log.Infof("Starting JSON only %s load request from %v for %s", runner, r.RemoteAddr, url)
		} else {
			log.Infof("Starting %s load request from %v for %s", runner, r.RemoteAddr, url)
		}
	} else {
		if r.FormValue("stop") == "Stop" {
			runid, _ = strconv.ParseInt(r.FormValue("runid"), 10, 64)
			log.Critf("Stop request from %v for %d", r.RemoteAddr, runid)
			mode = stop
		}
	}
	// Those only exist/make sense on run mode but go variable declaration...
	payload := r.FormValue("payload")
	labels := r.FormValue("labels")
	resolution, _ := strconv.ParseFloat(r.FormValue("r"), 64)
	percList, _ := stats.ParsePercentiles(r.FormValue("p"))
	qps, _ := strconv.ParseFloat(r.FormValue("qps"), 64)
	durStr := r.FormValue("t")
	jitter := (r.FormValue("jitter") == "on")
	grpcSecure := (r.FormValue("grpc-secure") == "on")
	grpcPing := (r.FormValue("ping") == "on")
	grpcPingDelay, _ := time.ParseDuration(r.FormValue("grpc-ping-delay"))
	stdClient := (r.FormValue("stdclient") == "on")
	httpsInsecure := (r.FormValue("https-insecure") == "on")
	resolve := r.FormValue("resolve")
	timeoutStr := strings.TrimSpace(r.FormValue("timeout"))
	timeout, _ := time.ParseDuration(timeoutStr) // will be 0 if empty, which is handled by runner and opts
	var dur time.Duration
	if durStr == "on" || ((len(r.Form["t"]) > 1) && r.Form["t"][1] == "on") {
		dur = -1
	} else {
		var err error
		dur, err = time.ParseDuration(durStr)
		if mode == run && err != nil {
			log.Errf("Error parsing duration '%s': %v", durStr, err)
		}
	}
	c, _ := strconv.Atoi(r.FormValue("c"))
	flusher, ok := w.(http.Flusher)
	if !ok {
		log.Fatalf("expected http.ResponseWriter to be an http.Flusher")
	}
	out := io.Writer(os.Stderr)
	if len(percList) == 0 && !strings.Contains(r.URL.RawQuery, "p=") {
		percList = defaultPercentileList
	}
	if !JSONOnly {
		out = fhttp.NewHTMLEscapeWriter(w)
	}
	n, _ := strconv.ParseInt(r.FormValue("n"), 10, 64)
	if strings.TrimSpace(url) == "" {
		url = "http://url.needed" // just because url validation doesn't like empty urls
	}
	ro := periodic.RunnerOptions{
		QPS:         qps,
		Duration:    dur,
		Out:         out,
		NumThreads:  c,
		Resolution:  resolution,
		Percentiles: percList,
		Labels:      labels,
		Exactly:     n,
		Jitter:      jitter,
	}
	if mode == run {
		ro.Normalize()
		uiRunMapMutex.Lock()
		id++ // start at 1 as 0 means interrupt all
		runid = id
		runs[runid] = &ro
		uiRunMapMutex.Unlock()
		log.Infof("New run id %d", runid)
	}
	httpopts := fhttp.NewHTTPOptions(url)
	defaultHeaders := httpopts.AllHeaders()
	httpopts.ResetHeaders()
	httpopts.DisableFastClient = stdClient
	httpopts.Insecure = httpsInsecure
	httpopts.Resolve = resolve
	httpopts.HTTPReqTimeOut = timeout
	if len(payload) > 0 {
		httpopts.Payload = []byte(payload)
	}
	if !JSONOnly {
		// Normal html mode
		if mainTemplate == nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Critf("Nil template")
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		durSeconds := dur.Seconds()
		if n > 0 {
			if qps > 0 {
				durSeconds = float64(n) / qps
			} else {
				durSeconds = -1
			}
			log.Infof("Estimating fixed #call %d duration to %g seconds %g", n, durSeconds, qps)
		}
		err := mainTemplate.Execute(w, &struct {
			R                           *http.Request
			Headers                     http.Header
			Version                     string
			LogoPath                    string
			DebugPath                   string
			ChartJSPath                 string
			StartTime                   string
			TargetURL                   string
			Labels                      string
			RunID                       int64
			UpTime                      time.Duration
			TestExpectedDurationSeconds float64
			URLHostPort                 string
			DoStop                      bool
			DoLoad                      bool
		}{
			r, defaultHeaders, version.Short(), logoPath, debugPath, chartJSPath,
			startTime.Format(time.ANSIC), url, labels, runid,
			fhttp.RoundDuration(time.Since(startTime)), durSeconds, urlHostPort, mode == stop, mode == run,
		})
		if err != nil {
			log.Critf("Template execution failed: %v", err)
		}
	}
	switch mode {
	case menu:
		// nothing more to do
	case stop:
		if runid <= 0 { // Stop all
			i := 0
			uiRunMapMutex.Lock()
			for _, v := range runs {
				v.Abort()
				i++
			}
			uiRunMapMutex.Unlock()
			log.Infof("Interrupted all %d runs", i)
		} else { // Stop one
			uiRunMapMutex.Lock()
			v, found := runs[runid]
			if found {
				v.Abort()
			}
			uiRunMapMutex.Unlock()
		}
	case run:
		// mode == run case:
		for _, header := range r.Form["H"] {
			if len(header) == 0 {
				continue
			}
			log.LogVf("adding header %v", header)
			err := httpopts.AddAndValidateExtraHeader(header)
			if err != nil {
				log.Errf("Error adding custom headers: %v", err)
			}
		}
		fhttp.OnBehalfOf(httpopts, r)
		if !JSONOnly {
			flusher.Flush()
		}
		var res periodic.HasRunnerResult
		var err error
		if runner == modegrpc {
			o := fgrpc.GRPCRunnerOptions{
				RunnerOptions: ro,
				Destination:   url,
				UsePing:       grpcPing,
				Delay:         grpcPingDelay,
				Insecure:      httpsInsecure,
			}
			if grpcSecure {
				o.Destination = fhttp.AddHTTPS(url)
			}
			// TODO: ReqTimeout: timeout
			res, err = fgrpc.RunGRPCTest(&o)
		} else if strings.HasPrefix(url, tcprunner.TCPURLPrefix) {
			// TODO: copy pasta from fortio_main
			o := tcprunner.RunnerOptions{
				RunnerOptions: ro,
			}
			o.ReqTimeout = timeout
			o.Destination = url
			o.Payload = httpopts.Payload
			res, err = tcprunner.RunTCPTest(&o)
		} else if strings.HasPrefix(url, udprunner.UDPURLPrefix) {
			// TODO: copy pasta from fortio_main
			o := udprunner.RunnerOptions{
				RunnerOptions: ro,
			}
			o.ReqTimeout = timeout
			o.Destination = url
			o.Payload = httpopts.Payload
			res, err = udprunner.RunUDPTest(&o)
		} else {
			o := fhttp.HTTPRunnerOptions{
				HTTPOptions:        *httpopts,
				RunnerOptions:      ro,
				AllowInitialErrors: true,
			}
			res, err = fhttp.RunHTTPTest(&o)
		}
		if err != nil {
			log.Errf("Init error for %s mode with url %s and options %+v : %v", runner, url, ro, err)

			_, _ = w.Write([]byte(fmt.Sprintf(
				"❌ Aborting because of %s\n</pre><script>document.getElementById('running').style.display = 'none';</script></body></html>\n",
				html.EscapeString(err.Error()))))
			return
		}
		json, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			log.Fatalf("Unable to json serialize result: %v", err)
		}
		savedAs := ""
		id := res.Result().ID()
		if DoSave {
			savedAs = SaveJSON(id, json)
		}
		if JSONOnly {
			w.Header().Set("Content-Type", "application/json")
			_, err = w.Write(json)
			if err != nil {
				log.Errf("Unable to write json output for %v: %v", r.RemoteAddr, err)
			}
			return
		}
		if savedAs != "" {
			_, _ = w.Write([]byte(fmt.Sprintf("Saved result to <a href='%s'>%s</a>"+
				" (<a href='browse?url=%s.json' target='_new'>graph link</a>)\n", savedAs, savedAs, id)))
		}

		_, _ = w.Write([]byte(fmt.Sprintf("All done %d calls %.3f ms avg, %.1f qps\n</pre>\n<script>\n",
			res.Result().DurationHistogram.Count,
			1000.*res.Result().DurationHistogram.Avg,
			res.Result().ActualQPS)))
		ResultToJsData(w, json)
		_, _ = w.Write([]byte("</script><p>Go to <a href='./'>Top</a>.</p></body></html>\n"))
		delete(runs, runid)
	}
}

// ResultToJsData converts a result object to chart data arrays and title
// and creates a chart from the result object.
func ResultToJsData(w io.Writer, json []byte) {
	_, _ = w.Write([]byte("var res = "))
	_, _ = w.Write(json)
	_, _ = w.Write([]byte("\nvar data = fortioResultToJsChartData(res)\nshowChart(data)\n"))
}

// SaveJSON save Json bytes to give file name (.json) in data-path dir.
func SaveJSON(name string, json []byte) string {
	if dataDir == "" {
		log.Infof("Not saving because data-path is unset")
		return ""
	}
	name += ".json"
	log.Infof("Saving %s in %s", name, dataDir)
	err := ioutil.WriteFile(path.Join(dataDir, name), json, 0o644) // nolint: gosec // we do want 644
	if err != nil {
		log.Errf("Unable to save %s in %s: %v", name, dataDir, err)
		return ""
	}
	// Return the relative path from the /fortio/ UI
	return "data/" + name
}

// SelectableValue represets an entry in the <select> of results.
type SelectableValue struct {
	Value    string
	Selected bool
}

// SelectValues maps the list of values (from DataList) to a list of SelectableValues.
// Each returned SelectableValue is selected if its value is contained in selectedValues.
// It is assumed that values does not contain duplicates.
func SelectValues(values []string, selectedValues []string) (selectableValues []SelectableValue, numSelected int) {
	set := make(map[string]bool, len(selectedValues))
	for _, selectedValue := range selectedValues {
		set[selectedValue] = true
	}

	for _, value := range values {
		_, selected := set[value]
		if selected {
			numSelected++
			delete(set, value)
		}
		selectableValue := SelectableValue{Value: value, Selected: selected}
		selectableValues = append(selectableValues, selectableValue)
	}
	return selectableValues, numSelected
}

// DataList returns the .json files/entries in data dir.
func DataList() (dataList []string) {
	files, err := ioutil.ReadDir(dataDir)
	if err != nil {
		log.Critf("Can list directory %s: %v", dataDir, err)
		return
	}
	// Newest files at the top:
	for i := len(files) - 1; i >= 0; i-- {
		name := files[i].Name()
		ext := ".json"
		if !strings.HasSuffix(name, ext) || files[i].IsDir() {
			log.LogVf("Skipping non %s file: %s", ext, name)
			continue
		}
		dataList = append(dataList, name[:len(name)-len(ext)])
	}
	log.LogVf("data list is %v (out of %d files in %s)", dataList, len(files), dataDir)
	return dataList
}

// ChartOptions describes the user-configurable options for a chart.
type ChartOptions struct {
	XMin   string
	XMax   string
	XIsLog bool
	YIsLog bool
}

// BrowseHandler handles listing and rendering the JSON results.
func BrowseHandler(w http.ResponseWriter, r *http.Request) {
	fhttp.LogRequest(r, "Browse")
	path := r.URL.Path
	if (path != uiPath) && (path != (uiPath + "browse")) {
		if strings.HasPrefix(path, "/fortio") {
			log.Infof("Redirecting /fortio in browse only path '%s'", path)
			http.Redirect(w, r, uiPath, http.StatusSeeOther)
		} else {
			log.Infof("Illegal browse path '%s'", path)
			w.WriteHeader(http.StatusNotFound)
		}
		return
	}
	url := r.FormValue("url")
	search := r.FormValue("s")
	xMin := r.FormValue("xMin")
	xMax := r.FormValue("xMax")
	// Ignore error, xLog == nil is the same as xLog being unspecified.
	xLog, _ := strconv.ParseBool(r.FormValue("xLog"))
	yLog, _ := strconv.ParseBool(r.FormValue("yLog"))
	dataList := DataList()
	selectedValues := r.URL.Query()["sel"]
	preselectedDataList, numSelected := SelectValues(dataList, selectedValues)

	doRender := url != ""
	doSearch := search != ""
	doLoadSelected := doSearch || numSelected > 0
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")

	chartOptions := ChartOptions{
		XMin:   xMin,
		XMax:   xMax,
		XIsLog: xLog,
		YIsLog: yLog,
	}
	err := browseTemplate.Execute(w, &struct {
		R                   *http.Request
		Extra               string
		Version             string
		LogoPath            string
		ChartJSPath         string
		URL                 string
		Search              string
		ChartOptions        ChartOptions
		PreselectedDataList []SelectableValue
		URLHostPort         string
		DoRender            bool
		DoSearch            bool
		DoLoadSelected      bool
	}{
		r, extraBrowseLabel, version.Short(), logoPath, chartJSPath,
		url, search, chartOptions, preselectedDataList, urlHostPort,
		doRender, doSearch, doLoadSelected,
	})
	if err != nil {
		log.Critf("Template execution failed: %v", err)
	}
}

// LogAndAddCacheControl logs the request and wrapps an HTTP handler to add a Cache-Control header for static files.
func LogAndAddCacheControl(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fhttp.LogRequest(r, "Static")
		path := r.URL.Path
		if path == faviconPath {
			r.URL.Path = "/static/img" + faviconPath // fortio/version expected to be stripped already
			log.LogVf("Changed favicon internal path to %s", r.URL.Path)
		}
		fhttp.CacheOn(w)
		h.ServeHTTP(w, r)
	})
}

func sendHTMLDataIndex(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	_, _ = w.Write([]byte("<html><body><ul>\n"))
	for _, e := range DataList() {
		_, _ = w.Write([]byte("<li><a href=\""))
		_, _ = w.Write([]byte(e))
		_, _ = w.Write([]byte(".json\">"))
		_, _ = w.Write([]byte(e))
		_, _ = w.Write([]byte("</a>\n"))
	}
	_, _ = w.Write([]byte("</ul></body></html>"))
}

type tsvCache struct {
	cachedDirTime time.Time
	cachedResult  []byte
}

var (
	gTSVCache      tsvCache
	gTSVCacheMutex = &sync.Mutex{}
)

// format for gcloud transfer
// https://cloud.google.com/storage/transfer/create-url-list
func sendTSVDataIndex(urlPrefix string, w http.ResponseWriter) {
	info, err := os.Stat(dataDir)
	if err != nil {
		log.Errf("Unable to stat %s: %v", dataDir, err)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	gTSVCacheMutex.Lock() // Kind of a long time to hold a lock... hopefully the FS doesn't hang...
	useCache := (info.ModTime() == gTSVCache.cachedDirTime) && (len(gTSVCache.cachedResult) > 0)
	if !useCache {
		var b bytes.Buffer
		b.Write([]byte("TsvHttpData-1.0\n"))
		for _, e := range DataList() {
			fname := e + ".json"
			f, err := os.Open(path.Join(dataDir, fname))
			if err != nil {
				log.Errf("Open error for %s: %v", fname, err)
				continue
			}
			// nolint: gosec // This isn't a crypto hash, more like a checksum - and mandated by the spec above, not our choice
			h := md5.New()
			var sz int64
			if sz, err = io.Copy(h, f); err != nil {
				f.Close()
				log.Errf("Copy/read error for %s: %v", fname, err)
				continue
			}
			b.Write([]byte(urlPrefix))
			b.Write([]byte(fname))
			b.Write([]byte("\t"))
			b.Write([]byte(strconv.FormatInt(sz, 10)))
			b.Write([]byte("\t"))
			b.Write([]byte(base64.StdEncoding.EncodeToString(h.Sum(nil))))
			b.Write([]byte("\n"))
		}
		gTSVCache.cachedDirTime = info.ModTime()
		gTSVCache.cachedResult = b.Bytes()
	}
	result := gTSVCache.cachedResult
	lastModified := gTSVCache.cachedDirTime.Format(http.TimeFormat)
	gTSVCacheMutex.Unlock()
	log.Infof("Used cached %v to serve %d bytes TSV", useCache, len(result))
	w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
	// Cloud transfer requires ETag
	w.Header().Set("ETag", fmt.Sprintf("\"%s\"", lastModified))
	w.Header().Set("Last-Modified", lastModified)
	_, _ = w.Write(result)
}

// LogAndFilterDataRequest logs the data request.
func LogAndFilterDataRequest(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fhttp.LogRequest(r, "Data")
		path := r.URL.Path
		if strings.HasSuffix(path, "/") || strings.HasSuffix(path, "/index.html") {
			sendHTMLDataIndex(w)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		ext := "/index.tsv"
		if strings.HasSuffix(path, ext) { // nolint: nestif
			// Ingress effect:
			urlPrefix := baseURL
			if len(urlPrefix) == 0 {
				// The Host header includes original host/port, only missing is the proto:
				proto := r.Header.Get("X-Forwarded-Proto")
				if len(proto) == 0 {
					proto = "http"
				}
				urlPrefix = proto + "://" + r.Host + path[:len(path)-len(ext)+1]
			} else {
				urlPrefix += uiPath + "data/" // base has been cleaned of trailing / in fortio_main
			}
			log.Infof("Prefix is '%s'", urlPrefix)
			sendTSVDataIndex(urlPrefix, w)
			return
		}
		if !strings.HasSuffix(path, ".json") {
			log.Warnf("Filtering request for non .json '%s'", path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fhttp.CacheOn(w)
		h.ServeHTTP(w, r)
	})
}

// TODO: move tsv/xml sync handling to their own file (and possibly package)

// http.ResponseWriter + Flusher emulator - if we refactor the code this should
// not be needed. on the other hand it's useful and could be reused.
type outHTTPWriter struct {
	CodePtr *int // Needed because that interface is somehow pass by value
	Out     io.Writer
	header  http.Header
}

func (o outHTTPWriter) Header() http.Header {
	return o.header
}

func (o outHTTPWriter) Write(b []byte) (int, error) {
	return o.Out.Write(b)
}

func (o outHTTPWriter) WriteHeader(code int) {
	*o.CodePtr = code
	_, _ = o.Out.Write([]byte(fmt.Sprintf("\n*** result code: %d\n", code)))
}

func (o outHTTPWriter) Flush() {
	// nothing
}

// Sync is the non http equivalent of fortio/sync?url=u.
func Sync(out io.Writer, u string, datadir string) bool {
	dataDir = datadir
	v := url.Values{}
	v.Set("url", u)
	// TODO: better context?
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/sync-function?"+v.Encode(), nil)
	code := http.StatusOK // default
	w := outHTTPWriter{Out: out, CodePtr: &code}
	SyncHandler(w, req)
	return (code == http.StatusOK)
}

// SyncHandler handles syncing/downloading from tsv url.
func SyncHandler(w http.ResponseWriter, r *http.Request) {
	fhttp.LogRequest(r, "Sync")
	flusher, ok := w.(http.Flusher)
	if !ok {
		log.Fatalf("expected http.ResponseWriter to be an http.Flusher")
	}
	uStr := strings.TrimSpace(r.FormValue("url"))
	if syncTemplate != nil {
		err := syncTemplate.Execute(w, &struct {
			Version  string
			LogoPath string
			URL      string
		}{version.Short(), logoPath, uStr})
		if err != nil {
			log.Critf("Sync template execution failed: %v", err)
		}
	}
	_, _ = w.Write([]byte("Fetch of index/bucket url ... "))
	flusher.Flush()
	o := fhttp.NewHTTPOptions(uStr)
	fhttp.OnBehalfOf(o, r)
	// If we had hundreds of thousands of entry we should stream, parallelize (connection pool)
	// and not do multiple passes over the same data, but for small tsv this is fine.
	// use std client to change the url and handle https:
	client, _ := fhttp.NewStdClient(o)
	if client == nil {
		_, _ = w.Write([]byte("invalid url!<script>setPB(1,1)</script></body></html>\n"))
		w.WriteHeader(422 /*Unprocessable Entity*/)
		return
	}
	code, data, _ := client.Fetch()
	defer client.Close()
	if code != http.StatusOK {
		_, _ = w.Write([]byte(fmt.Sprintf("http error, code %d<script>setPB(1,1)</script></body></html>\n", code)))
		w.WriteHeader(424 /*Failed Dependency*/)
		return
	}
	sdata := strings.TrimSpace(string(data))
	if strings.HasPrefix(sdata, "TsvHttpData-1.0") {
		processTSV(w, client, sdata)
	} else {
		if !processXML(w, client, data, uStr, 0) {
			return
		}
	}
	_, _ = w.Write([]byte("</table>"))
	_, _ = w.Write([]byte("\n</body></html>\n"))
}

func processTSV(w http.ResponseWriter, client *fhttp.Client, sdata string) {
	flusher := w.(http.Flusher)
	lines := strings.Split(sdata, "\n")
	n := len(lines)

	_, _ = w.Write([]byte(fmt.Sprintf("success tsv fetch! Now fetching %d referenced URLs:<script>setPB(1,%d)</script>\n",
		n-1, n)))
	_, _ = w.Write([]byte("<table>"))
	flusher.Flush()
	for i, l := range lines[1:] {
		parts := strings.Split(l, "\t")
		u := parts[0]
		_, _ = w.Write([]byte("<tr><td>"))
		_, _ = w.Write([]byte(template.HTMLEscapeString(u)))
		ur, err := url.Parse(u)
		if err != nil {
			_, _ = w.Write([]byte("<td>skipped (not a valid url)"))
		} else {
			uPath := ur.Path
			pathParts := strings.Split(uPath, "/")
			name := pathParts[len(pathParts)-1]
			downloadOne(w, client, name, u)
		}
		_, _ = w.Write([]byte(fmt.Sprintf("</tr><script>setPB(%d)</script>\n", i+2)))
		flusher.Flush()
	}
}

// ListBucketResult is the minimum we need out of s3 xml results.
// https://docs.aws.amazon.com/AmazonS3/latest/API/RESTBucketGET.html
// e.g. https://storage.googleapis.com/fortio-data?max-keys=2&prefix=fortio.istio.io/
type ListBucketResult struct {
	NextMarker string   `xml:"NextMarker"`
	Names      []string `xml:"Contents>Key"`
}

// @returns true if started a table successfully - false is error.
func processXML(w http.ResponseWriter, client *fhttp.Client, data []byte, baseURL string, level int) bool {
	// We already know this parses as we just fetched it:
	bu, _ := url.Parse(baseURL)
	flusher := w.(http.Flusher)
	l := ListBucketResult{}
	err := xml.Unmarshal(data, &l)
	if err != nil {
		log.Errf("xml unmarshal error %v", err)
		// don't show the error / would need html escape to avoid CSS attacks
		_, _ = w.Write([]byte("❌ xml parsing error, check logs<script>setPB(1,1)</script></body></html>\n"))
		w.WriteHeader(http.StatusInternalServerError)
		return false
	}
	n := len(l.Names)
	log.Infof("Parsed %+v", l)

	_, _ = w.Write([]byte(fmt.Sprintf("success xml fetch #%d! Now fetching %d referenced objects:<script>setPB(1,%d)</script>\n",
		level+1, n, n+1)))
	if level == 0 {
		_, _ = w.Write([]byte("<table>"))
	}
	for i, el := range l.Names {
		_, _ = w.Write([]byte("<tr><td>"))
		_, _ = w.Write([]byte(template.HTMLEscapeString(el)))
		pathParts := strings.Split(el, "/")
		name := pathParts[len(pathParts)-1]
		newURL := *bu // copy
		newURL.Path = newURL.Path + "/" + el
		fullURL := newURL.String()
		downloadOne(w, client, name, fullURL)
		_, _ = w.Write([]byte(fmt.Sprintf("</tr><script>setPB(%d)</script>\n", i+2)))
		flusher.Flush()
	}
	flusher.Flush()
	// Is there more data ? (NextMarker present)
	if len(l.NextMarker) == 0 {
		return true
	}
	if level > 100 {
		log.Errf("Too many chunks, stopping after 100")
		w.WriteHeader(509 /* Bandwidth Limit Exceeded */)
		return true
	}
	q := bu.Query()
	if q.Get("marker") == l.NextMarker {
		log.Errf("Loop with same marker %+v", bu)
		w.WriteHeader(508 /* Loop Detected */)
		return true
	}
	q.Set("marker", l.NextMarker)
	bu.RawQuery = q.Encode()
	newBaseURL := bu.String()
	// url already validated
	_, _ = w.Write([]byte("<tr><td>"))
	_, _ = w.Write([]byte(template.HTMLEscapeString(newBaseURL)))
	_, _ = w.Write([]byte("<td>"))
	_ = client.ChangeURL(newBaseURL)
	ncode, ndata, _ := client.Fetch()
	if ncode != http.StatusOK {
		log.Errf("Can't fetch continuation with marker %+v", bu)

		_, _ = w.Write([]byte(fmt.Sprintf("❌ http error, code %d<script>setPB(1,1)</script></table></body></html>\n", ncode)))
		w.WriteHeader(424 /*Failed Dependency*/)
		return false
	}
	return processXML(w, client, ndata, newBaseURL, level+1) // recurse
}

func downloadOne(w http.ResponseWriter, client *fhttp.Client, name string, u string) {
	log.Infof("downloadOne(%s,%s)", name, u)
	if !strings.HasSuffix(name, ".json") {
		_, _ = w.Write([]byte("<td>skipped (not json)"))
		return
	}
	localPath := path.Join(dataDir, name)
	_, err := os.Stat(localPath)
	if err == nil {
		_, _ = w.Write([]byte("<td>skipped (already exists)"))
		return
	}
	// note that if data dir doesn't exist this will trigger too - TODO: check datadir earlier
	if !os.IsNotExist(err) {
		log.Warnf("check %s : %v", localPath, err)
		// don't return the details of the error to not leak local data dir etc
		_, _ = w.Write([]byte("<td>❌ skipped (access error)"))
		return
	}
	// url already validated
	_ = client.ChangeURL(u)
	code1, data1, _ := client.Fetch()
	if code1 != http.StatusOK {
		_, _ = w.Write([]byte(fmt.Sprintf("<td>❌ Http error, code %d", code1)))
		w.WriteHeader(424 /*Failed Dependency*/)
		return
	}
	err = ioutil.WriteFile(localPath, data1, 0o644) // nolint: gosec // we do want 644
	if err != nil {
		log.Errf("Unable to save %s: %v", localPath, err)
		_, _ = w.Write([]byte("<td>❌ skipped (write error)"))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	// finally ! success !
	log.Infof("Success fetching %s - saved at %s", u, localPath)
	// checkmark
	_, _ = w.Write([]byte("<td class='checkmark'>✓"))
}

// Serve starts the fhttp.Serve() plus the UI server on the given port
// and paths (empty disables the feature). uiPath should end with /
// (be a 'directory' path). Returns true if server is started successfully.
func Serve(baseurl, port, debugpath, uipath, staticRsrcDir string, datadir string, percentileList []float64) bool {
	baseURL = baseurl
	startTime = time.Now()
	mux, addr := fhttp.Serve(port, debugpath)
	if addr == nil {
		return false // Error already logged
	}
	if uipath == "" {
		return true
	}
	fhttp.SetupPPROF(mux)
	uiPath = uipath
	dataDir = datadir
	if uiPath[len(uiPath)-1] != '/' {
		log.Warnf("Adding missing trailing / to UI path '%s'", uiPath)
		uiPath += "/"
	}
	debugPath = ".." + debugpath // TODO: calculate actual path if not same number of directories
	mux.HandleFunc(uiPath, Handler)
	fetchPath = uiPath + fetchURI
	// For backward compatibility with http:// only fetcher
	mux.Handle(fetchPath, http.StripPrefix(fetchPath, http.HandlerFunc(fhttp.FetcherHandler)))
	// h2 incoming and https outgoing ok fetcher
	mux.HandleFunc(uiPath+fetch2URI, fhttp.FetcherHandler2)
	fhttp.CheckConnectionClosedHeader = true // needed for proxy to avoid errors

	logoPath = version.Short() + "/static/img/logo.svg"
	chartJSPath = version.Short() + "/static/js/Chart.min.js"

	// Serve static contents in the ui/static dir. If not otherwise specified
	// by the function parameter staticPath, we use getResourcesDir which uses the
	// link time value or the directory relative to this file to find the static
	// contents, so no matter where or how the go binary is generated, the static
	// dir should be found.
	staticRsrcDir = getResourcesDir(staticRsrcDir)
	if staticRsrcDir != "" { // nolint: nestif
		fs := http.FileServer(http.Dir(staticRsrcDir))
		prefix := uiPath + version.Short()
		mux.Handle(prefix+"/static/", LogAndAddCacheControl(http.StripPrefix(prefix, fs)))
		mux.Handle(faviconPath, LogAndAddCacheControl(fs))
		var err error
		mainTemplate, err = template.ParseFiles(path.Join(staticRsrcDir, "templates/main.html"),
			path.Join(staticRsrcDir, "templates/header.html"))
		if err != nil {
			log.Critf("Unable to parse main template: %v", err)
		}
		browseTemplate, err = template.ParseFiles(path.Join(staticRsrcDir, "templates/browse.html"),
			path.Join(staticRsrcDir, "templates/header.html"))
		if err != nil {
			log.Critf("Unable to parse browse template: %v", err)
		} else {
			mux.HandleFunc(uiPath+"browse", BrowseHandler)
		}
		syncTemplate, err = template.ParseFiles(path.Join(staticRsrcDir, "templates/sync.html"),
			path.Join(staticRsrcDir, "templates/header.html"))
		if err != nil {
			log.Critf("Unable to parse sync template: %v", err)
		} else {
			mux.HandleFunc(uiPath+"sync", SyncHandler)
		}
		dflagSetURL := uiPath + "flags/set"
		dflagEndPt := endpoint.NewFlagsEndpoint(flag.CommandLine, dflagSetURL)
		mux.HandleFunc(uiPath+"flags", dflagEndPt.ListFlags)
		mux.HandleFunc(dflagSetURL, dflagEndPt.SetFlag)
	}
	if dataDir != "" {
		fs := http.FileServer(http.Dir(dataDir))
		mux.Handle(uiPath+"data/", LogAndFilterDataRequest(http.StripPrefix(uiPath+"data", fs)))
		if datadir == "." {
			var err error
			datadir, err = os.Getwd()
			if err != nil {
				log.Errf("Unable to get current directory: %v", err)
				datadir = dataDir
			}
		}
		fmt.Println("Data directory is", datadir)
	}
	urlHostPort = fnet.NormalizeHostPort(port, addr)
	uiMsg := "UI started - visit:\n"
	if strings.Contains(urlHostPort, "-unix-socket=") {
		uiMsg += fmt.Sprintf("fortio curl %s http://localhost%s", urlHostPort, uiPath)
	} else {
		uiMsg += fmt.Sprintf("http://%s%s", urlHostPort, uiPath)
		if strings.Contains(urlHostPort, "localhost") {
			uiMsg += "\n(or any host/ip reachable on this server)"
		}
	}
	fmt.Println(uiMsg)
	defaultPercentileList = percentileList
	return true
}

// Report starts the browsing only UI server on the given port.
// Similar to Serve with only the read only part.
func Report(baseurl, port, staticRsrcDir string, datadir string) bool {
	// drop the pprof default handlers [shouldn't be needed with custom mux but better safe than sorry]
	http.DefaultServeMux = http.NewServeMux()
	baseURL = baseurl
	extraBrowseLabel = ", report only limited UI"
	mux, addr := fhttp.HTTPServer("report", port)
	if addr == nil {
		return false
	}
	urlHostPort = fnet.NormalizeHostPort(port, addr)
	uiMsg := fmt.Sprintf("Browse only UI started - visit:\nhttp://%s/", urlHostPort)
	if !strings.Contains(port, ":") {
		uiMsg += "   (or any host/ip reachable on this server)"
	}
	fmt.Printf(uiMsg + "\n")
	uiPath = "/"
	dataDir = datadir
	logoPath = version.Short() + "/static/img/logo.svg"
	chartJSPath = version.Short() + "/static/js/Chart.min.js"
	staticRsrcDir = getResourcesDir(staticRsrcDir)
	fs := http.FileServer(http.Dir(staticRsrcDir))
	prefix := uiPath + version.Short()
	mux.Handle(prefix+"/static/", LogAndAddCacheControl(http.StripPrefix(prefix, fs)))
	mux.Handle(faviconPath, LogAndAddCacheControl(fs))
	var err error
	browseTemplate, err = template.ParseFiles(path.Join(staticRsrcDir, "templates/browse.html"),
		path.Join(staticRsrcDir, "templates/header.html"))
	if err != nil {
		log.Critf("Unable to parse browse template: %v", err)
	} else {
		mux.HandleFunc(uiPath, BrowseHandler)
	}
	fsd := http.FileServer(http.Dir(dataDir))
	mux.Handle(uiPath+"data/", LogAndFilterDataRequest(http.StripPrefix(uiPath+"data", fsd)))
	return true
}
