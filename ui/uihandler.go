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

// TODO: break down into functions to generate the x,y data that can be unit tested
// and a function to process the result and produce the graph and one for the UI/form
// and triggering the run separately.

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
		const templ = `<!DOCTYPE html><html><head><title>Φορτίο v{{.Version}}</title>
<script src="{{.ChartJSPath}}"></script>
</head>
<body style="background: linear-gradient(to right, #d8aa20 , #c75228);">
<img src="{{.LogoPath}}" alt="." height="69" width="45" align="right" />
<img src="{{.LogoPath}}" alt="." height="92" width="60" align="right" />
<img src="{{.LogoPath}}" alt="istio logo" height="123" width="80" align="right" />
<h1>Φορτίο (fortio) v{{.Version}}{{if not .DoLoad}} control UI{{end}}</h1>
<p>Up for {{.UpTime}} (since {{.StartTime}}).
{{if .DoLoad}}
<p>{{.Labels}} {{.TargetURL}}
<br />
<div class="chart-container" style="position: relative; height:75vh; width:98vw">
<canvas style="background-color: #fff; visibility: hidden;" id="chart1"></canvas>
</div>
<div id="running">
<br/>
Running load test... Results pending...
</div>
<div id="update" style="visibility: hidden">
<form id="updtForm" action="javascript:updateChart()">
<input type="submit" value="Update:" />
Time axis min <input type="text" name="xmin" size="5" /> ms,
max <input type="text" name="xmax" size="5" /> ms,
logarithmic: <input name="xlog" type="checkbox" onclick="updateChart()" /> -
Count axis logarithmic: <input name="ylog" type="checkbox" onclick="updateChart()" />
</form>
</div>
<pre>
{{else}}
{{if .DoExit}}
<p>Exiting server as per request.</p>
{{else}}
<form>
<div>
Title/Labels: <input type="text" name="labels" size="40" value="Fortio" /> (empty to skip title)<br />
URL: <input type="text" name="url" size="60" value="http://localhost:{{.Port}}/echo" /> <br />
QPS: <input type="text" name="qps" size="6" value="1000" />
Duration: <input type="text" name="t" size="6" value="3s" /> <br />
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
<form action="javascript:document.location += 'fetch/' + document.getElementById('uri').value">
<div>
Debug fetch http://<input type="text" id="uri" name="uri" value="" size=50/>
</div>
</form>
<p><i>Or</i></p>
<form method="POST">
<div>
Use with caution, will interrupt/end this server: <input type="submit" name="exit" value="Exit" />
</div>
</form>
<p>See also <a href="{{.DebugPath}}">debug</a> and <a href="{{.DebugPath}}?env=dump">debug with env dump</a>.
{{end}}
</body>
</html>
{{end}}`
		t := template.Must(template.New("htmlOut").Parse(templ))
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		err := t.Execute(w, &struct {
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
		if DoLoad || DoExit {
			flusher.Flush()
		}
	}
	if DoLoad {
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
				w.Write([]byte(fmt.Sprintf("All done %d calls %.3f ms avg, %.1f qps\n</pre>\n<script>\n",
					res.DurationHistogram.Count,
					1000.*res.DurationHistogram.Avg,
					res.ActualQPS)))
				w.Write([]byte(`var dataP = [{x: 0.0, y: 0.0}, `)) // nolint: errcheck
				for i, it := range res.DurationHistogram.Data {
					var x float64
					if i == 0 {
						// Extra point, 1/N at min itself
						x = 1000. * it.Start
						// nolint: errcheck
						w.Write([]byte(fmt.Sprintf("{x: %.12g, y: %.3f},\n", x, 100./float64(res.DurationHistogram.Count))))
					}
					if i == len(res.DurationHistogram.Data)-1 {
						//last point we use the end part (max)
						x = 1000. * it.End
					} else {
						x = 1000. * (it.Start + it.End) / 2.
					}
					// nolint: errcheck
					w.Write([]byte(fmt.Sprintf("{x: %.12g, y: %.3f},\n", x, it.Percent)))
				}
				w.Write([]byte(`];var dataH = [`)) // nolint: errcheck
				prev := 1000. * res.DurationHistogram.Data[0].Start
				for _, it := range res.DurationHistogram.Data {
					startX := 1000. * it.Start
					endX := 1000. * it.End
					if startX != prev {
						w.Write([]byte(fmt.Sprintf("{x: %.12g, y: 0},{x: %.12g, y: 0},\n", prev, startX))) // nolint: errcheck
					}
					// nolint: errcheck
					w.Write([]byte(fmt.Sprintf("{x: %.12g, y: %d},{x: %.12g, y: %d},\n", startX, it.Count, endX, it.Count)))
					prev = endX
				}
				// nolint: errcheck
				w.Write([]byte(`];
document.getElementById('running').style.display='none';
var chartEl = document.getElementById('chart1');
chartEl.style.visibility='visible';
document.getElementById('update').style.visibility='visible';
var ctx = chartEl.getContext('2d');
var linearXAxe = {
 type: 'linear',
 scaleLabel : {
  display: true,
  labelString: 'Response time in ms',
  ticks: {
   min: 0,
   beginAtZero: true,
  },
 }
}
var logXAxe = {
  type: 'logarithmic',
  scaleLabel : {
   display: true,
   labelString: 'Response time in ms (log scale)'
  },
  ticks: {
    //min: dataH[0].x, // newer chart.js are ok with 0 on x axis too
    callback: function(tick, index, ticks) {return tick.toLocaleString()}
  }
}
var linearYAxe = {
       id: 'H',
       type: 'linear',
       ticks: {
        beginAtZero: true,
       },
       scaleLabel : {
        display: true,
        labelString: 'Count'
       }
      };
var logYAxe = {
 id: 'H',
 type: 'logarithmic',
 display: true,
 ticks: {
 // min: 1, // log mode works even with 0s
 // Needed to not get scientific notation display:
 callback: function(tick, index, ticks) {return tick.toString()}
 },
 scaleLabel : {
 display: true,
 labelString: 'Count (log scale)'
 }
}
var chart = new Chart(ctx, {
  type: 'line',
  data: {datasets: [
  {
   label: 'Cumulative %',
   data: dataP,
   fill: false,
   yAxisID: 'P',
   stepped: true,
   backgroundColor: 'rgba(134, 87, 167, 1)',
   borderColor: 'rgba(134, 87, 167, 1)',
  },
 {
   label: 'Histogram: Count',
   data: dataH,
   yAxisID: 'H',
   pointStyle: 'rect',
   radius: 1,
   borderColor: 'rgba(87, 167, 134, .9)',
   backgroundColor: 'rgba(87, 167, 134, .75)'
 }]},
  options: {
  responsive: true,
   maintainAspectRatio: false,
   title: {
    display: true,
    fontStyle: 'normal',
    text: [`))
				if res.Labels != "" {
					// nolint: errcheck
					w.Write([]byte(fmt.Sprintf("'%s - %s - %s',",
						res.Labels, res.URL, res.StartTime.Format(time.ANSIC)))) // TODO: escape single quote
				}
				percStr := fmt.Sprintf("min %.3f ms, average %.3f ms", 1000.*res.DurationHistogram.Min, 1000.*res.DurationHistogram.Avg)
				for _, p := range res.DurationHistogram.Percentiles {
					percStr += fmt.Sprintf(", p%g %.2f ms", p.Percentile, 1000*p.Value)
				}
				percStr += fmt.Sprintf(", max %.3f ms", 1000.*res.DurationHistogram.Max)
				// nolint: errcheck
				w.Write([]byte(fmt.Sprintf("'Response time histogram at %s target qps (%.1f actual) %d connections for %s (actual %v)','%s'",
					res.RequestedQPS, res.ActualQPS, res.NumThreads, res.RequestedDuration, fhttp.RoundDuration(res.ActualDuration),
					percStr)))
				// nolint: errcheck
				w.Write([]byte(`],
    },
    elements: {
     line: {
      tension: 0, // disables bezier curves
     }
    },
    scales: {
      xAxes: [
       linearXAxe
      ],
      yAxes: [{
       id: 'P',
       position: 'right',
       ticks: {
        beginAtZero: true,
       },
       scaleLabel : {
        display: true,
        labelString: '%'
       }
       },
      linearYAxe
      ]
    }
  }
});
function updateChart() {
	var form = document.getElementById('updtForm')
	var formMin = form.xmin.value.trim()
	var formMax = form.xmax.value.trim()
	var scales = chart.config.options.scales
	var newAxis
	var newXMin = parseFloat(formMin)
	if (form.xlog.checked) {
		newXAxis = logXAxe
		//if (formMin == "0") {
		//	newXMin = dataH[0].x // log doesn't like 0 xaxis
		//}
	} else {
		newXAxis = linearXAxe
	}
	if (form.ylog.checked) {
		chart.config.options.scales = {xAxes: [newXAxis], yAxes: [scales.yAxes[0], logYAxe]}
	} else {
		chart.config.options.scales = {xAxes: [newXAxis], yAxes: [scales.yAxes[0], linearYAxe]}
	}
	chart.update()
	var newNewXAxis = chart.config.options.scales.xAxes[0]
	if (formMin != "") {
		newNewXAxis.ticks.min = newXMin
	} else {
		delete newNewXAxis.ticks.min
	}
	if (formMax != "" && formMax != "max") {
		newNewXAxis.ticks.max = parseFloat(formMax)
	} else {
		delete newNewXAxis.ticks.max
	}
	chart.update()
}
</script>
</body></html>`))
			}
		}
	}
	if DoExit {
		syscall.Kill(syscall.Getpid(), syscall.SIGINT) // nolint: errcheck
	}
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

	logoPath = "./static/img/logo.svg"
	chartJSPath = "./static/js/Chart.min.js"

	// Serve static contents in the ui/static dir. If not otherwise specified
	// by the function parameter staticPath, we use getDataDir which uses the
	// link time value or the directory relative to this file to find the static
	// contents, so no matter where or how the go binary is generated, the static
	// dir should be found.
	staticPath = getDataDir(staticPath)
	if staticPath != "" {
		fs := http.FileServer(http.Dir(staticPath))
		http.Handle(uiPath+"static/", LogAndAddCacheControl(http.StripPrefix(uiPath, fs)))
	}
	fhttp.Serve(port, debugpath)
}
