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
	"html/template"
	"io"
	"net/http"
	"os"
	"path"
	"runtime"
	"strconv"
	"syscall"
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
	dataDirectory string
	mainTemplate  *template.Template
)

const (
	fetchURI = "fetch/"
)

// Gets the data directory from one of 3 sources:
func getDataDir(override string) string {
	if override != "" {
		log.Infof("Using data directory from override: %s", override)
		return override
	}
	if dataDirectory != "" {
		log.Infof("Using data directory set at link time: %s", dataDirectory)
		return dataDirectory
	}
	_, filename, _, ok := runtime.Caller(0)
	log.Infof("Guessing data directory from runtime source location: %v - %s", ok, filename)
	if ok {
		return path.Dir(filename)
	}
	log.Errf("Unable to get source tree location. Failing to serve static contents.")
	return ""
}

// TODO: auto map from (Http)RunnerOptions to form generation and/or accept
// JSON serialized options as input.

// TODO: unit tests, allow additional data sets.

// Handler is the UI handler creating the web forms and processing them.
func Handler(w http.ResponseWriter, r *http.Request) {
	LogRequest(r)
	DoExit := false
	if r.FormValue("exit") == "Exit" {
		log.Critf("Exit request from %v", r.RemoteAddr)
		DoExit = true
	}
	DoLoad := false
	JSONOnly := false
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
	if !JSONOnly {
		// Normal html mode
		if mainTemplate == nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Critf("Nil template")
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		err := mainTemplate.Execute(w, &struct {
			R           *http.Request
			Headers     http.Header
			Version     string
			LogoPath    string
			DebugPath   string
			ChartJSPath string
			StartTime   string
			TargetURL   string
			Labels      string
			UpTime      time.Duration
			Port        int
			DoExit      bool
			DoLoad      bool
		}{r, fhttp.GetHeaders(), periodic.Version, logoPath, debugPath, chartJSPath,
			startTime.Format(time.ANSIC), url, labels,
			fhttp.RoundDuration(time.Since(startTime)), httpPort, DoExit, DoLoad})
		if err != nil {
			log.Critf("Template execution failed: %v", err)
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			log.Fatalf("expected http.ResponseWriter to be an http.Flusher")
		}
		if !DoLoad && !DoExit {
			return
		}
		flusher.Flush()
	}
	if DoExit {
		syscall.Kill(syscall.Getpid(), syscall.SIGINT) // nolint: errcheck
		return
	}
	resolution, _ := strconv.ParseFloat(r.FormValue("r"), 64)
	percList, _ := stats.ParsePercentiles(r.FormValue("p"))
	qps, _ := strconv.ParseFloat(r.FormValue("qps"), 64)
	durStr := r.FormValue("t")
	dur, err := time.ParseDuration(durStr)
	if err != nil {
		log.Errf("Error parsing duration '%s': %v", durStr, err)
	}
	c, _ := strconv.Atoi(r.FormValue("c"))
	firstHeader := true
	for _, header := range r.Form["H"] {
		if len(header) == 0 {
			continue
		}
		log.LogVf("adding header %v", header)
		if firstHeader {
			// If there is at least 1 non empty H passed, reset the header list
			fhttp.ResetHeaders()
			firstHeader = false
		}
		err = fhttp.AddAndValidateExtraHeader(header)
		if err != nil {
			log.Errf("Error adding custom headers: %v", err)
		}
	}
	out := io.Writer(w)
	if JSONOnly {
		out = os.Stderr
	}
	ro := periodic.RunnerOptions{
		QPS:         qps,
		Duration:    dur,
		Out:         out,
		NumThreads:  c,
		Resolution:  resolution,
		Percentiles: percList,
		Labels:      labels,
	}
	o := fhttp.HTTPRunnerOptions{
		RunnerOptions: ro,
		URL:           url,
	}
	res, err := fhttp.RunHTTPTest(&o)
	if err != nil {
		w.Write([]byte(fmt.Sprintf("Aborting because %v\n", err))) // nolint: errcheck
		return
	}
	if JSONOnly {
		w.Header().Set("Content-Type", "application/json")
		j, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			log.Fatalf("Unable to json serialize result: %v", err)
		}
		_, err = w.Write(j)
		if err != nil {
			log.Errf("Unable to write json output for %v: %v", r.RemoteAddr, err)
		}
		return
	}
	// nolint: errcheck
	w.Write([]byte(fmt.Sprintf("All done %d calls %.3f ms avg, %.1f qps\n</pre>\n<script>\n",
		res.DurationHistogram.Count,
		1000.*res.DurationHistogram.Avg,
		res.ActualQPS)))
	ResultToJsData(w, res)
	ResultToChart(w, res)
	w.Write([]byte("</script></body></html>\n"))
}

// ResultToJsData converts a result object to chart data arrays.
func ResultToJsData(w io.Writer, res *fhttp.HTTPRunnerResults) {
	j, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		log.Fatalf("Unable to json serialize result: %v", err)
	}
	w.Write([]byte("var res = "))
	w.Write(j)
	w.Write([]byte("\nvar data = fortioResultToJsChartData(res)\n"))
}

// ResultToChart creates a chart from the result object
func ResultToChart(w io.Writer, res *fhttp.HTTPRunnerResults) {
	// nolint: errcheck
	w.Write([]byte("showChart(data)\n"))
}

// LogRequest logs the incoming request, including headers when loglevel is verbose
func LogRequest(r *http.Request) {
	log.Infof("%v %v %v %v", r.Method, r.URL, r.Proto, r.RemoteAddr)
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
		LogRequest(r)
		w.Header().Set("Cache-Control", "max-age=365000000, immutable")
		h.ServeHTTP(w, r)
	})
}

// FetcherHandler is the handler for the fetcher/proxy.
func FetcherHandler(w http.ResponseWriter, r *http.Request) {
	LogRequest(r)
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
	client := fhttp.NewBasicClient("http://"+url, "1.1",
		/* keepalive: */
		false,
		/* halfclose: */
		false)
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
func Serve(port int, debugpath, uipath, staticPath string) {
	startTime = time.Now()
	httpPort = port
	if uipath == "" {
		fhttp.Serve(port, debugpath) // doesn't return until exit
		return
	}
	uiPath = uipath
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
	// by the function parameter staticPath, we use getDataDir which uses the
	// link time value or the directory relative to this file to find the static
	// contents, so no matter where or how the go binary is generated, the static
	// dir should be found.
	staticPath = getDataDir(staticPath)
	if staticPath != "" {
		fs := http.FileServer(http.Dir(staticPath))
		prefix := uiPath + periodic.Version
		http.Handle(prefix+"/static/", LogAndAddCacheControl(http.StripPrefix(prefix, fs)))
		var err error
		mainTemplate, err = template.ParseFiles(path.Join(staticPath, "templates/main.html"))
		if err != nil {
			log.Critf("Unable to parse template: %v", err)
		}
	}
	fhttp.Serve(port, debugpath)
}
