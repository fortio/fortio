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

package fhttp // import "istio.io/fortio/fhttp"

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"syscall"
	"time"

	"istio.io/fortio/log"
	"istio.io/fortio/periodic"
)

// RoundDuration rounds to 10th of second. Only for positive durations.
// TODO: switch to Duration.Round once switched to go 1.9
func RoundDuration(d time.Duration) time.Duration {
	tenthSec := int64(100 * time.Millisecond)
	r := int64(d+50*time.Millisecond) / tenthSec
	return time.Duration(tenthSec * r)
}

// UIHandler is the UI handler
func UIHandler(w http.ResponseWriter, r *http.Request) {
	log.Infof("%v %v %v %v", r.Method, r.URL, r.Proto, r.RemoteAddr)
	DoExit := false
	if r.FormValue("exit") == "Exit" {
		log.Critf("Exit request from %v", r.RemoteAddr)
		DoExit = true
	}
	DoLoad := false
	if r.FormValue("load") == "Start" {
		log.Critf("Exit request from %v", r.RemoteAddr)
		DoLoad = true
	}
	/*
		data, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Errf("Error reading %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	*/
	const templ = `<!DOCTYPE html><html><head><title>Φορτίο (fortio) version {{.Version}}</title></head>
<body>
<h1>Φορτίο (fortio) version {{.Version}} 'UI'</h1>
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
QPS: <input type="text" name="qps" size="6" value="100" /> <br />
Duration: <input type="text" name="t" size="12" value="5s" /> <br />
<input type="submit" name="load" value="Start"/>
</div>
</form>
<p>Or</p>
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
		Version   string
		DebugPath string
		DoExit    bool
		UpTime    time.Duration
		StartTime string
		DoLoad    bool
		Port      int
	}{r, periodic.Version, debugPath, DoExit,
		RoundDuration(time.Since(startTime)), startTime.Format(time.UnixDate),
		DoLoad, httpPort})
	if err != nil {
		log.Critf("Template execution failed: %v", err)
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		log.Fatalf("expected http.ResponseWriter to be an http.Flusher")
	}
	if DoLoad {
		flusher.Flush()
		qps, _ := strconv.ParseFloat(r.FormValue("qps"), 64)
		dur, _ := time.ParseDuration(r.FormValue("t"))
		ro := periodic.RunnerOptions{
			QPS:      qps,
			Duration: dur,
		}
		o := HTTPRunnerOptions{
			RunnerOptions: ro,
			URL:           r.FormValue("url"),
		}
		res, err := RunHTTPTest(&o)
		if err != nil {
			w.Write([]byte(fmt.Sprintf("Aborting because %v\n", err)))
		} else {
			w.Write([]byte(fmt.Sprintf("All done %d calls %.3f ms avg, %.1f qps\n</pre><hr /><pre>\n",
				res.Result().DurationHistogram.Count,
				1000.*res.Result().DurationHistogram.Avg,
				res.Result().ActualQPS)))
			j, err := json.MarshalIndent(res.Result(), "", "  ")
			if err != nil {
				log.Fatalf("Unable to json serialize result: %v", err)
			}
			w.Write(j)
			w.Write([]byte("\n</pre></body></html>\n"))
		}
	}
	if DoExit {
		flusher.Flush()
		syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	}
}
