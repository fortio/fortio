// Copyright 2017 Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

// Do not add any external dependencies we want to keep fortio minimal.

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"runtime"

	"istio.io/fortio/fgrpc"
	"istio.io/fortio/fhttp"
	"istio.io/fortio/log"
	"istio.io/fortio/periodic"
	"istio.io/fortio/stats"
	"istio.io/fortio/ui"
)

var httpOpts fhttp.HTTPOptions

func init() {
	httpOpts.Init("")
}

// -- Support for multiple instances of -H flag on cmd line:
type flagList struct {
}

// Unclear when/why this is called and necessary
func (f *flagList) String() string {
	return ""
}
func (f *flagList) Set(value string) error {
	return httpOpts.AddAndValidateExtraHeader(value)
}

// -- end of functions for -H support

// Prints usage
func usage(msgs ...interface{}) {
	// nolint: gas
	fmt.Fprintf(os.Stderr, "Φορτίο %s usage:\n\t%s command [flags] target\n%s\n%s\n%s\n",
		periodic.Version,
		os.Args[0],
		"where command is one of: load (load testing), server (starts grpc ping and http echo/ui servers), grpcping (grpc client)",
		"where target is a url (http load tests) or host:port (grpc health test)",
		"and flags are:")
	flag.PrintDefaults()
	fmt.Fprint(os.Stderr, msgs...) // nolint: gas
	os.Stderr.WriteString("\n")    // nolint: gas, errcheck
	os.Exit(1)
}

var (
	defaults = &periodic.DefaultRunnerOptions
	// Very small default so people just trying with random URLs don't affect the target
	qpsFlag         = flag.Float64("qps", defaults.QPS, "Queries Per Seconds or 0 for no wait/max qps")
	numThreadsFlag  = flag.Int("c", defaults.NumThreads, "Number of connections/goroutine/threads")
	durationFlag    = flag.Duration("t", defaults.Duration, "How long to run the test or 0 to run until ^C")
	percentilesFlag = flag.String("p", "50,75,99,99.9", "List of pXX to calculate")
	resolutionFlag  = flag.Float64("r", defaults.Resolution, "Resolution of the histogram lowest buckets in seconds")
	compressionFlag = flag.Bool("compression", false, "Enable http compression")
	goMaxProcsFlag  = flag.Int("gomaxprocs", 0, "Setting for runtime.GOMAXPROCS, <1 doesn't change the default")
	profileFlag     = flag.String("profile", "", "write .cpu and .mem profiles to file")
	keepAliveFlag   = flag.Bool("keepalive", true, "Keep connection alive (only for fast http 1.1)")
	halfCloseFlag   = flag.Bool("halfclose", false, "When not keepalive, whether to half close the connection (only for fast http)")
	stdClientFlag   = flag.Bool("stdclient", false, "Use the slower net/http standard client (works for TLS)")
	http10Flag      = flag.Bool("http1.0", false, "Use http1.0 (instead of http 1.1)")
	grpcFlag        = flag.Bool("grpc", false, "Use GRPC (health check) for load testing")
	echoPortFlag    = flag.Int("http-port", 8080, "http echo server port")
	grpcPortFlag    = flag.Int("grpc-port", 8079, "grpc port")
	echoDbgPathFlag = flag.String("echo-debug-path", "/debug",
		"http echo server URI for debug, empty turns off that part (more secure)")
	jsonFlag       = flag.String("json", "", "Json output to provided file or '-' for stdout (empty = no json output)")
	uiPathFlag     = flag.String("ui-path", "/fortio/", "http server URI for UI, empty turns off that part (more secure)")
	curlFlag       = flag.Bool("curl", false, "Just fetch the content once")
	labelsFlag     = flag.String("labels", "", "Additional config data/labels to add to the resulting JSON, defaults to hostname")
	staticPathFlag = flag.String("static-path", "", "Absolute path to the dir containing the static files dir")

	headersFlags flagList
	percList     []float64
	err          error
)

func main() {
	flag.Var(&headersFlags, "H", "Additional Header(s)")
	flag.IntVar(&fhttp.BufferSizeKb, "httpbufferkb", fhttp.BufferSizeKb,
		"Size of the buffer (max data size) for the optimized http client in kbytes")
	flag.BoolVar(&fhttp.CheckConnectionClosedHeader, "httpccch", fhttp.CheckConnectionClosedHeader,
		"Check for Connection: Close Header")
	if len(os.Args) < 2 {
		usage("Error: need at least 1 command parameter")
	}
	command := os.Args[1]
	os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
	flag.Parse()
	percList, err = stats.ParsePercentiles(*percentilesFlag)
	if err != nil {
		usage("Unable to extract percentiles from -p: ", err)
	}

	switch command {
	case "load":
		fortioLoad()
	case "server":
		go ui.Serve(*echoPortFlag, *echoDbgPathFlag, *uiPathFlag, *staticPathFlag)
		pingServer(*grpcPortFlag)
	case "grpcping":
		grpcClient()
	default:
		usage("Error: unknown command ", command)
	}
}

func fetchURL(o *fhttp.HTTPOptions) {
	// keepAlive could be just false when making 1 fetch but it helps debugging
	// the http client when making a single request if using the flags
	client := fhttp.NewClient(o)
	if client == nil {
		return // error logged already
	}
	code, data, header := client.Fetch()
	log.LogVf("Fetch result code %d, data len %d, headerlen %d", code, len(data), header)
	os.Stdout.Write(data) //nolint: errcheck
	if code != http.StatusOK {
		log.Errf("Error status %d : %s", code, fhttp.DebugSummary(data, 512))
		os.Exit(1)
	}
}

func fortioLoad() {
	if len(flag.Args()) != 1 {
		usage("Error: fortio load needs a url or destination")
	}
	url := flag.Arg(0)
	httpOpts.URL = url
	httpOpts.HTTP10 = *http10Flag
	httpOpts.DisableFastClient = *stdClientFlag
	httpOpts.DisableKeepAlive = !*keepAliveFlag
	httpOpts.AllowHalfClose = *halfCloseFlag
	httpOpts.Compression = *compressionFlag
	if *curlFlag {
		fetchURL(&httpOpts)
		return
	}
	prevGoMaxProcs := runtime.GOMAXPROCS(*goMaxProcsFlag)
	out := os.Stderr
	fmt.Printf("Fortio %s running at %g queries per second, %d->%d procs",
		periodic.Version, *qpsFlag, prevGoMaxProcs, runtime.GOMAXPROCS(0))
	if *durationFlag <= 0 {
		// Infinite mode is determined by having a negative duration value
		*durationFlag = -1
		fmt.Printf(", until interrupted: %s\n", url)
	} else {
		fmt.Printf(", for %v: %s\n", *durationFlag, url)
	}
	qps := *qpsFlag
	if qps <= 0 {
		qps = -1 // 0==unitialized struct == default duration, -1 (0 for flag) is max
	}
	labels := *labelsFlag
	if labels == "" {
		labels, _ = os.Hostname()
	}
	ro := periodic.RunnerOptions{
		QPS:         qps,
		Duration:    *durationFlag,
		NumThreads:  *numThreadsFlag,
		Percentiles: percList,
		Resolution:  *resolutionFlag,
		Out:         out,
		Labels:      labels,
	}
	var res periodic.HasRunnerResult
	if *grpcFlag {
		o := fgrpc.GRPCRunnerOptions{
			RunnerOptions: ro,
			Destination:   url,
		}
		res, err = fgrpc.RunGRPCTest(&o)
	} else {
		o := fhttp.HTTPRunnerOptions{
			RunnerOptions: ro,
			Profiler:      *profileFlag,
		}
		res, err = fhttp.RunHTTPTest(&o)
	}
	if err != nil {
		fmt.Fprintf(out, "Aborting because %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(out, "All done %d calls (plus %d warmup) %.3f ms avg, %.1f qps\n",
		res.Result().DurationHistogram.Count,
		*numThreadsFlag,
		1000.*res.Result().DurationHistogram.Avg,
		res.Result().ActualQPS)
	jsonFileName := *jsonFlag
	if len(jsonFileName) > 0 {
		j, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			log.Fatalf("Unable to json serialize result: %v", err)
		}
		var f *os.File
		if jsonFileName == "-" {
			f = os.Stdout
			jsonFileName = "stdout"
		} else {
			f, err = os.Create(jsonFileName)
			if err != nil {
				log.Fatalf("Unable to create %s: %v", jsonFileName, err)
			}
		}
		n, err := f.Write(append(j, '\n'))
		if err != nil {
			log.Fatalf("Unable to write json to %s: %v", jsonFileName, err)
		}
		if f != os.Stdout {
			err := f.Close()
			if err != nil {
				log.Fatalf("Close error for %s: %v", jsonFileName, err)
			}
		}
		fmt.Fprintf(out, "Successfully wrote %d bytes of Json data to %s\n", n, jsonFileName)
	}
}
