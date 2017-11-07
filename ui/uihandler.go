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

// Adapted from istio/proxy/test/backend/echo with error handling and
// concurrency fixes and making it as low overhead as possible
// (no std output by default)

package ui // import "istio.io/fortio/ui"

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
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
	logoPath  string
	debugPath string
	// Used to construct default URL to self.
	httpPort int
	// Start time of the UI Server (for uptime info).
	startTime time.Time
)

// TODO: auto map from (Http)RunnerOptions to form generation and/or accept
// JSON serialized options as input.

// Handler is the UI handler creating the web forms and processing them.
func Handler(w http.ResponseWriter, r *http.Request) {
	log.Infof("%v %v %v %v", r.Method, r.URL, r.Proto, r.RemoteAddr)
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
	if !JSONOnly {
		// Normal html mode
		const templ = `<!DOCTYPE html><html><head><title>Φορτίο v{{.Version}} control UI</title></head>
<body style="background: linear-gradient(to right, #d8aa20 , #c75228);">
<img src="{{.LogoPath}}" alt="." height="69" width="45" align="right" />
<img src="{{.LogoPath}}" alt="." height="92" width="60" align="right" />
<img src="{{.LogoPath}}" alt="istio logo" height="123" width="80" align="right" />
<h1>Φορτίο (fortio) v{{.Version}} control UI</h1>
<p>
Up for {{.UpTime}} (since {{.StartTime}})
</p>
<p>
{{if .DoLoad}}
Running load test ...
<pre>
{{else}}
{{if .DoExit}}
<p>Exiting server as per request.</p>
{{else}}
<form>
<div>
URL: <input type="text" name="url" size="60" value="http://localhost:{{.Port}}/echo" /> <br />
QPS: <input type="text" name="qps" size="6" value="1000" />
Duration: <input type="text" name="t" size="6" value="5s" /> <br />
Threads/Simultaneous connections: <input type="text" name="c" size="6" value="8" /> <br />
Percentiles: <input type="text" name="p" size="20" value="50, 75, 99, 99.9" /> <br />
Histogram Resolution: <input type="text" name="r" size="8" value="0.0001" /> <br />
Headers: <br />
{{ range $name, $vals := .Headers }}{{range $val := $vals}}
<input type="text" name="H" size=40 value="{{$name}}: {{ $val }}" /> <br />
{{end}}{{end}}
<input type="text" name="H" size=40 value="" /> <br />
JSON output: <input type="checkbox" name="json" /> <br />
<input type="submit" name="load" value="Start"/>
</div>
</form>
<p><i>Or</i></p>
<form method="POST">
<div>
Use with caution, will end this server: <input type="submit" name="exit" value="Exit" />
</div>
</form>
<p>See also <a href="{{.DebugPath}}">debug</a> and <a href="{{.DebugPath}}?env=dump">debug with env dump</a>.
{{end}}
</body>
</html>
{{end}}
`
		t := template.Must(template.New("htmlOut").Parse(templ))
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		err := t.Execute(w, &struct {
			R         *http.Request
			Headers   http.Header
			Version   string
			LogoPath  string
			DebugPath string
			StartTime string
			UpTime    time.Duration
			Port      int
			DoExit    bool
			DoLoad    bool
		}{r, fhttp.GetHeaders(), periodic.Version, logoPath, debugPath,
			startTime.Format(time.UnixDate), fhttp.RoundDuration(time.Since(startTime)),
			httpPort, DoExit, DoLoad})
		if err != nil {
			log.Critf("Template execution failed: %v", err)
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			log.Fatalf("expected http.ResponseWriter to be an http.Flusher")
		}
		if DoLoad || DoExit {
			flusher.Flush()
		}
	}
	if DoLoad {
		resolution, _ := strconv.ParseFloat(r.FormValue("r"), 64)
		percList, _ := stats.ParsePercentiles(r.FormValue("p"))
		qps, _ := strconv.ParseFloat(r.FormValue("qps"), 64)
		dur, _ := time.ParseDuration(r.FormValue("t"))
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
			err := fhttp.AddAndValidateExtraHeader(header)
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
		}
		o := fhttp.HTTPRunnerOptions{
			RunnerOptions: ro,
			URL:           url,
		}
		res, err := fhttp.RunHTTPTest(&o)
		if err != nil {
			w.Write([]byte(fmt.Sprintf("Aborting because %v\n", err))) // nolint: errcheck
		} else {
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
			} else {
				// nolint: errcheck
				w.Write([]byte(fmt.Sprintf("All done %d calls %.3f ms avg, %.1f qps\n</pre></body></html>\n",
					res.Result().DurationHistogram.Count,
					1000.*res.Result().DurationHistogram.Avg,
					res.Result().ActualQPS)))
			}
		}
	}
	if DoExit {
		syscall.Kill(syscall.Getpid(), syscall.SIGINT) // nolint: errcheck
	}
}

// LogoHandler is the handler for the logo
func LogoHandler(w http.ResponseWriter, r *http.Request) {
	log.Infof("%v %v %v %v", r.Method, r.URL, r.Proto, r.RemoteAddr)
	w.Header().Set("Content-Type", "image/svg+xml")
	// nolint: errcheck, lll
	w.Write([]byte(`<svg viewBox="0 0 424 650" xmlns="http://www.w3.org/2000/svg"><g fill="#fff"><path d="M422 561l-292 79-118-79 411 0Zm-282-350v280l-138 45 138-325ZM173 11l0 480 250 47-250-527Z"/></g></svg>`))
}

// Serve starts the fhttp.Serve() plus the UI server on the given port
// and paths (empty disables the feature).
func Serve(port int, debugpath string, uiPath string) {
	debugPath = debugpath
	startTime = time.Now()
	httpPort = port
	if uiPath != "" {
		http.HandleFunc(uiPath, Handler)
		fmt.Printf("UI starting - visit:\nhttp://localhost:%d%s\n", port, uiPath)
		logoPath = uiPath + "/logo.svg"
		http.HandleFunc(logoPath, LogoHandler)
	}
	fhttp.Serve(port, debugpath)
}
