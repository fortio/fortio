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
	mutex          = &sync.Mutex{}
	id             int64
	runs           = make(map[int64]chan struct{})
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

// Handler is the main UI handler creating the web forms and processing them.
func Handler(w http.ResponseWriter, r *http.Request) {
	LogRequest(r, "UI")
	DoStop := false
	runid, _ := strconv.ParseInt(r.FormValue("runid"), 10, 64) // nolint: gas
	if r.FormValue("stop") == "Stop" {
		log.Critf("Stop request from %v for %d", r.RemoteAddr, runid)
		DoStop = true
	}
	DoLoad := false
	JSONOnly := false
	DoSave := (r.FormValue("save") == "on")
	url := r.FormValue("url")
	if r.FormValue("load") == "Start" {
		DoLoad = true
		if r.FormValue("json") == "on" {
			JSONOnly = true
			log.Infof("Starting JSON only load request from %v for %s", r.RemoteAddr, url)
		} else {
			log.Infof("Starting load request from %v for %s", r.RemoteAddr, url)
		}
	}
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
		if DoLoad && err != nil {
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
	thisID := int64(0)
	if DoLoad {
		ro.Normalize()
		mutex.Lock()
		id++ // start at 1 as 0 means interrupt all
		thisID = id
		runs[thisID] = ro.Stop
		mutex.Unlock()
		log.Infof("New run id %d", thisID)
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
			startTime.Format(time.ANSIC), url, labels, thisID,
			fhttp.RoundDuration(time.Since(startTime)), dur.Seconds(), httpPort, DoStop, DoLoad})
		if err != nil {
			log.Critf("Template execution failed: %v", err)
		}

		if !DoLoad && !DoStop {
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			log.Fatalf("expected http.ResponseWriter to be an http.Flusher")
		}
		flusher.Flush()
	}
	if DoStop {
		if runid <= 0 { // Stop all
			i := 0
			mutex.Lock()
			for _, v := range runs {
				close(v)
				i++
			}
			mutex.Unlock()
			log.Infof("Interrupted %d runs", i)
		} else { // Stop one
			mutex.Lock()
			c := runs[runid]
			if c != nil {
				close(c)
			}
			mutex.Unlock()
		}

		return
	}
	// DoLoad case:
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
	res, err := fhttp.RunHTTPTest(&o)
	mutex.Lock()
	delete(runs, thisID)
	mutex.Unlock()
	if err != nil {
		w.Write([]byte(fmt.Sprintf("Aborting because %s\n", html.EscapeString(err.Error())))) // nolint: errcheck,gas
		return
	}
	json, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		log.Fatalf("Unable to json serialize result: %v", err)
	}
	if JSONOnly {
		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write(json)
		if err != nil {
			log.Errf("Unable to write json output for %v: %v", r.RemoteAddr, err)
		}
		return
	}
	if DoSave {
		res := SaveJSON(res.ID(), json)
		if res != "" {
			// nolint: errcheck, gas
			w.Write([]byte(fmt.Sprintf("Saved result to <a href='%s'>%s</a>\n", res, res)))
		}
	}
	// nolint: errcheck, gas
	w.Write([]byte(fmt.Sprintf("All done %d calls %.3f ms avg, %.1f qps\n</pre>\n<script>\n",
		res.DurationHistogram.Count,
		1000.*res.DurationHistogram.Avg,
		res.ActualQPS)))
	ResultToJsData(w, json)
	w.Write([]byte("</script></body></html>\n")) // nolint: gas
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

// BrowseHandler handles listing and rendering the JSON results.
func BrowseHandler(w http.ResponseWriter, r *http.Request) {
	LogRequest(r, "Browse")
	url := r.FormValue("url")
	doRender := (url != "")
	files, err := ioutil.ReadDir(dataDir)
	if err != nil {
		log.Critf("Can list directory %s: %v", dataDir, err)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	var dataList []string
	// Newest files at the top:
	for i := len(files) - 1; i >= 0; i-- {
		name := files[i].Name()
		ext := ".json"
		if !strings.HasSuffix(name, ext) {
			log.LogVf("Skipping non %s file: %s", ext, name)
			continue
		}
		dataList = append(dataList, name[:len(name)-len(ext)])
	}
	log.Infof("data list is %v", dataList)
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	err = browseTemplate.Execute(w, &struct {
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

// LogDataRequest logs the data request.
func LogDataRequest(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		LogRequest(r, "Data")
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
	opts.DisableKeepAlive = true
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
		fhttp.Serve(port, debugpath) // doesn't return until stop
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
		http.Handle(uiPath+"data/", LogDataRequest(http.StripPrefix(uiPath+"data", fs)))
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
	http.Handle(uiPath+"data/", LogDataRequest(http.StripPrefix(uiPath+"data", fsd)))
	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil); err != nil {
		log.Critf("Error starting server: %v", err)
	}
}
