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

package ui // import "istio.io/fortio/ui"

import (
	"bytes"
	// md5 is mandated, not our choice
	"crypto/md5" // nolint: gas
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"istio.io/fortio/fhttp"
	"istio.io/fortio/log"
	"istio.io/fortio/periodic"
	"istio.io/fortio/stats"
)

var (
	// UI and Debug prefix/paths (read in ui handler).
	uiPath      string // absolute (base)
	logoPath    string // relative
	chartJSPath string // relative
	debugPath   string // mostly relative
	fetchPath   string // this one is absolute
	// Used to construct default URL to self.
	httpPort int
	// Start time of the UI Server (for uptime info).
	startTime time.Time
	// Directory where the static content and templates are to be loaded from.
	// This is replaced at link time to the packaged directory (e.g /usr/local/lib/fortio/)
	// but when fortio is installed with go get we use RunTime to find that directory.
	// (see Dockerfile for how to set it)
	resourcesDir     string
	extraBrowseLabel string // Extra label for report only
	// Directory where results are written to/read from
	dataDir        string
	mainTemplate   *template.Template
	browseTemplate *template.Template
	uiRunMapMutex  = &sync.Mutex{}
	id             int64
	runs           = make(map[int64]*periodic.RunnerOptions)
)

const (
	fetchURI = "fetch/"
)

// Gets the resources directory from one of 3 sources:
func getResourcesDir(override string) string {
	if override != "" {
		log.Infof("Using resources directory from override: %s", override)
		return override
	}
	if resourcesDir != "" {
		log.Infof("Using resources directory set at link time: %s", resourcesDir)
		return resourcesDir
	}
	_, filename, _, ok := runtime.Caller(0)
	log.Infof("Guessing resources directory from runtime source location: %v - %s", ok, filename)
	if ok {
		return path.Dir(filename)
	}
	log.Errf("Unable to get source tree location. Failing to serve static contents.")
	return ""
}

// HTMLEscapeWriter is an io.Writer escaping the output for safe html inclusion.
type HTMLEscapeWriter struct {
	NextWriter io.Writer
}

func (w *HTMLEscapeWriter) Write(p []byte) (int, error) {
	template.HTMLEscape(w.NextWriter, p)
	return len(p), nil
}

// TODO: auto map from (Http)RunnerOptions to form generation and/or accept
// JSON serialized options as input.

// TODO: unit tests, allow additional data sets.

type mode int

// The main html has 3 principal modes:
const (
	// Default: renders the forms/menus
	menu mode = iota
	// Trigger a run
	run
	// Request abort
	stop
)

// Handler is the main UI handler creating the web forms and processing them.
func Handler(w http.ResponseWriter, r *http.Request) {
	LogRequest(r, "UI")
	mode := menu
	JSONOnly := false
	DoSave := (r.FormValue("save") == "on")
	url := r.FormValue("url")
	runid := int64(0)
	if r.FormValue("load") == "Start" {
		mode = run
		if r.FormValue("json") == "on" {
			JSONOnly = true
			log.Infof("Starting JSON only load request from %v for %s", r.RemoteAddr, url)
		} else {
			log.Infof("Starting load request from %v for %s", r.RemoteAddr, url)
		}
	} else {
		if r.FormValue("stop") == "Stop" {
			runid, _ = strconv.ParseInt(r.FormValue("runid"), 10, 64) // nolint: gas
			log.Critf("Stop request from %v for %d", r.RemoteAddr, runid)
			mode = stop
		}
	}
	// Those only exist/make sense on run mode but go variable declaration...
	labels := r.FormValue("labels")
	resolution, _ := strconv.ParseFloat(r.FormValue("r"), 64) // nolint: gas
	percList, _ := stats.ParsePercentiles(r.FormValue("p"))   // nolint: gas
	qps, _ := strconv.ParseFloat(r.FormValue("qps"), 64)      // nolint: gas
	durStr := r.FormValue("t")
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
	c, _ := strconv.Atoi(r.FormValue("c")) // nolint: gas
	out := io.Writer(os.Stderr)
	if !JSONOnly {
		out = io.Writer(&HTMLEscapeWriter{NextWriter: w})
	}
	opts := fhttp.NewHTTPOptions(url)
	ro := periodic.RunnerOptions{
		QPS:         qps,
		Duration:    dur,
		Out:         out,
		NumThreads:  c,
		Resolution:  resolution,
		Percentiles: percList,
		Labels:      labels,
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
	if !JSONOnly {
		// Normal html mode
		if mainTemplate == nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Critf("Nil template")
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
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
			Port                        int
			DoStop                      bool
			DoLoad                      bool
		}{r, opts.GetHeaders(), periodic.Version, logoPath, debugPath, chartJSPath,
			startTime.Format(time.ANSIC), url, labels, runid,
			fhttp.RoundDuration(time.Since(startTime)), dur.Seconds(), httpPort, mode == stop, mode == run})
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
			log.Infof("Interrupted %d runs", i)
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
		firstHeader := true
		for _, header := range r.Form["H"] {
			if len(header) == 0 {
				continue
			}
			log.LogVf("adding header %v", header)
			if firstHeader {
				// If there is at least 1 non empty H passed, reset the header list
				opts.ResetHeaders()
				firstHeader = false
			}
			err := opts.AddAndValidateExtraHeader(header)
			if err != nil {
				log.Errf("Error adding custom headers: %v", err)
			}
		}
		o := fhttp.HTTPRunnerOptions{
			RunnerOptions:      ro,
			HTTPOptions:        *opts,
			AllowInitialErrors: true,
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			log.Fatalf("expected http.ResponseWriter to be an http.Flusher")
		}
		if !JSONOnly {
			flusher.Flush()
		}
		res, err := fhttp.RunHTTPTest(&o)
		uiRunMapMutex.Lock()
		delete(runs, runid)
		uiRunMapMutex.Unlock()
		if err != nil {
			w.Write([]byte(fmt.Sprintf("Aborting because %s\n", html.EscapeString(err.Error())))) // nolint: errcheck,gas
			return
		}
		json, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			log.Fatalf("Unable to json serialize result: %v", err)
		}
		savedAs := ""
		if DoSave {
			savedAs = SaveJSON(res.ID(), json)
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
			// nolint: errcheck, gas
			w.Write([]byte(fmt.Sprintf("Saved result to <a href='%s'>%s</a>\n", savedAs, savedAs)))
		}
		// nolint: errcheck, gas
		w.Write([]byte(fmt.Sprintf("All done %d calls %.3f ms avg, %.1f qps\n</pre>\n<script>\n",
			res.DurationHistogram.Count,
			1000.*res.DurationHistogram.Avg,
			res.ActualQPS)))
		ResultToJsData(w, json)
		w.Write([]byte("</script></body></html>\n")) // nolint: gas
	}
}

// ResultToJsData converts a result object to chart data arrays and title
// and creates a chart from the result object
func ResultToJsData(w io.Writer, json []byte) {
	// nolint: errcheck, gas
	w.Write([]byte("var res = "))
	// nolint: errcheck, gas
	w.Write(json)
	// nolint: errcheck, gas
	w.Write([]byte("\nvar data = fortioResultToJsChartData(res)\nshowChart(data)\n"))
}

// SaveJSON save Json bytes to give file name (.json) in data-path dir.
func SaveJSON(name string, json []byte) string {
	if dataDir == "" {
		log.Infof("Not saving because data-path is unset")
		return ""
	}
	name += ".json"
	log.Infof("Saving %s in %s", name, dataDir)
	err := ioutil.WriteFile(path.Join(dataDir, name), json, 0644)
	if err != nil {
		log.Errf("Unable to save %s in %s: %v", name, dataDir, err)
		return ""
	}
	// Return the relative path from the /fortio/ UI
	return "data/" + name
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
	log.LogVf("data list is %v", dataList)
	return dataList
}

// BrowseHandler handles listing and rendering the JSON results.
func BrowseHandler(w http.ResponseWriter, r *http.Request) {
	LogRequest(r, "Browse")
	url := r.FormValue("url")
	doRender := (url != "")
	dataList := DataList()
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	err := browseTemplate.Execute(w, &struct {
		R           *http.Request
		Extra       string
		Version     string
		LogoPath    string
		ChartJSPath string
		URL         string
		DataList    []string
		Port        int
		DoRender    bool
	}{r, extraBrowseLabel, periodic.Version, logoPath, chartJSPath,
		url, dataList, httpPort, doRender})
	if err != nil {
		log.Critf("Template execution failed: %v", err)
	}
}

// LogRequest logs the incoming request, including headers when loglevel is verbose
func LogRequest(r *http.Request, msg string) {
	log.Infof("%s: %v %v %v %v", msg, r.Method, r.URL, r.Proto, r.RemoteAddr)
	if log.LogVerbose() {
		for name, headers := range r.Header {
			for _, h := range headers {
				log.LogVf("Header %v: %v\n", name, h)
			}
		}
	}
}

// LogAndAddCacheControl logs the request and wrapps an HTTP handler to add a Cache-Control header for static files.
func LogAndAddCacheControl(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		LogRequest(r, "Static")
		w.Header().Set("Cache-Control", "max-age=365000000, immutable")
		h.ServeHTTP(w, r)
	})
}

func sendHTMLDataIndex(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	w.Write([]byte("<html><body><ul>\n")) // nolint: errcheck, gas
	for _, e := range DataList() {
		w.Write([]byte("<li><a href=\"")) // nolint: errcheck, gas
		w.Write([]byte(e))                // nolint: errcheck, gas
		w.Write([]byte(".json\">"))       // nolint: errcheck, gas
		w.Write([]byte(e))                // nolint: errcheck, gas
		w.Write([]byte("</a>\n"))         // nolint: errcheck, gas
	}
	w.Write([]byte("</ul></body></html>")) // nolint: errcheck, gas
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
		b.Write([]byte("TsvHttpData-1.0\n")) // nolint: errcheck, gas
		for _, e := range DataList() {
			fname := e + ".json"
			f, err := os.Open(path.Join(dataDir, fname))
			if err != nil {
				log.Errf("Open error for %s: %v", fname, err)
				continue
			}
			// This isn't a crypto hash, more like a checksum - and mandated by the
			// spec above, not our choice
			h := md5.New() // nolint: gas
			var sz int64
			if sz, err = io.Copy(h, f); err != nil {
				f.Close() // nolint: errcheck, gas
				log.Errf("Copy/read error for %s: %v", fname, err)
				continue
			}
			b.Write([]byte(urlPrefix))                                     // nolint: errcheck, gas
			b.Write([]byte(fname))                                         // nolint: errcheck, gas
			b.Write([]byte("\t"))                                          // nolint: errcheck, gas
			b.Write([]byte(strconv.FormatInt(sz, 10)))                     // nolint: errcheck, gas
			b.Write([]byte("\t"))                                          // nolint: errcheck, gas
			b.Write([]byte(base64.StdEncoding.EncodeToString(h.Sum(nil)))) // nolint: errcheck, gas
			b.Write([]byte("\n"))                                          // nolint: errcheck, gas
		}
		gTSVCache.cachedDirTime = info.ModTime()
		gTSVCache.cachedResult = b.Bytes()
	}
	result := gTSVCache.cachedResult
	gTSVCacheMutex.Unlock()
	log.Infof("Used cached %v to serve %d bytes TSV", useCache, len(result))
	w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
	w.Write(result) // nolint: errcheck, gas
}

// LogAndFilterDataRequest logs the data request.
func LogAndFilterDataRequest(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		LogRequest(r, "Data")
		path := r.URL.Path
		if strings.HasSuffix(path, "/") || strings.HasSuffix(path, "/index.html") {
			sendHTMLDataIndex(w)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		ext := "/index.tsv"
		if strings.HasSuffix(path, ext) {
			// TODO: what if we are reached through https ingress? or a different port
			urlPrefix := "http://" + r.Host + path[:len(path)-len(ext)+1]
			log.LogVf("Prefix is '%s'", urlPrefix)
			sendTSVDataIndex(urlPrefix, w)
			return
		}
		if !strings.HasSuffix(path, ".json") {
			log.Warnf("Filtering request for non .json '%s'", path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// FetcherHandler is the handler for the fetcher/proxy.
func FetcherHandler(w http.ResponseWriter, r *http.Request) {
	LogRequest(r, "Fetch")
	hj, ok := w.(http.Hijacker)
	if !ok {
		log.Critf("hijacking not supported")
		return
	}
	conn, _, err := hj.Hijack()
	if err != nil {
		log.Errf("hijacking error %v", err)
		return
	}
	// Don't forget to close the connection:
	defer conn.Close() // nolint: errcheck
	url := r.URL.String()[len(fetchPath):]
	opts := fhttp.NewHTTPOptions("http://" + url)
	opts.HTTPReqTimeOut = 5 * time.Minute
	client := fhttp.NewClient(opts)
	if client == nil {
		return // error logged already
	}
	_, data, _ := client.Fetch()
	_, err = conn.Write(data)
	if err != nil {
		log.Errf("Error writing fetched data to %v: %v", r.RemoteAddr, err)
	}
}

// Serve starts the fhttp.Serve() plus the UI server on the given port
// and paths (empty disables the feature). uiPath should end with /
// (be a 'directory' path)
func Serve(port int, debugpath, uipath, staticRsrcDir string, datadir string) {
	startTime = time.Now()
	httpPort = port
	if uipath == "" {
		fhttp.Serve(port, debugpath) // doesn't return until exit
		return
	}
	uiPath = uipath
	dataDir = datadir
	if uiPath[len(uiPath)-1] != '/' {
		log.Warnf("Adding missing trailing / to UI path '%s'", uiPath)
		uiPath += "/"
	}
	debugPath = ".." + debugpath // TODO: calculate actual path if not same number of directories
	http.HandleFunc(uiPath, Handler)
	fmt.Printf("UI starting - visit:\nhttp://localhost:%d%s\n", port, uiPath)

	fetchPath = uiPath + fetchURI
	http.HandleFunc(fetchPath, FetcherHandler)
	fhttp.CheckConnectionClosedHeader = true // needed for proxy to avoid errors

	logoPath = periodic.Version + "/static/img/logo.svg"
	chartJSPath = periodic.Version + "/static/js/Chart.min.js"

	// Serve static contents in the ui/static dir. If not otherwise specified
	// by the function parameter staticPath, we use getResourcesDir which uses the
	// link time value or the directory relative to this file to find the static
	// contents, so no matter where or how the go binary is generated, the static
	// dir should be found.
	staticRsrcDir = getResourcesDir(staticRsrcDir)
	if staticRsrcDir != "" {
		fs := http.FileServer(http.Dir(staticRsrcDir))
		prefix := uiPath + periodic.Version
		http.Handle(prefix+"/static/", LogAndAddCacheControl(http.StripPrefix(prefix, fs)))
		var err error
		mainTemplate, err = template.ParseFiles(path.Join(staticRsrcDir, "templates/main.html"))
		if err != nil {
			log.Critf("Unable to parse main template: %v", err)
		}
		browseTemplate, err = template.ParseFiles(path.Join(staticRsrcDir, "templates/browse.html"))
		if err != nil {
			log.Critf("Unable to parse browse template: %v", err)
		} else {
			http.HandleFunc(uiPath+"browse", BrowseHandler)
		}
	}
	if dataDir != "" {
		fs := http.FileServer(http.Dir(dataDir))
		http.Handle(uiPath+"data/", LogAndFilterDataRequest(http.StripPrefix(uiPath+"data", fs)))
	}
	fhttp.Serve(port, debugpath)
}

// Report starts the browsing only UI server on the given port.
// Similar to Serve with only the read only part.
func Report(port int, staticRsrcDir string, datadir string) {
	extraBrowseLabel = ", report only limited UI"
	httpPort = port
	uiPath = "/"
	dataDir = datadir
	fmt.Printf("Browse only UI starting - visit:\nhttp://localhost:%d/\n", port)
	logoPath = periodic.Version + "/static/img/logo.svg"
	chartJSPath = periodic.Version + "/static/js/Chart.min.js"
	staticRsrcDir = getResourcesDir(staticRsrcDir)
	fs := http.FileServer(http.Dir(staticRsrcDir))
	prefix := uiPath + periodic.Version
	http.Handle(prefix+"/static/", LogAndAddCacheControl(http.StripPrefix(prefix, fs)))
	var err error
	browseTemplate, err = template.ParseFiles(path.Join(staticRsrcDir, "templates/browse.html"))
	if err != nil {
		log.Critf("Unable to parse browse template: %v", err)
	} else {
		http.HandleFunc(uiPath, BrowseHandler)
	}
	fsd := http.FileServer(http.Dir(dataDir))
	http.Handle(uiPath+"data/", LogAndFilterDataRequest(http.StripPrefix(uiPath+"data", fsd)))
	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil); err != nil {
		log.Critf("Error starting server: %v", err)
	}
}
