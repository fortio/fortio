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

// Originally fortio_main.go at the root of fortio.orf/fortio
// now moved here it can be customized and reused in variants of fortio
// like fortiotel (fortio with opentelemetry)
package cli // import "fortio.org/fortio/cli"

// Do not add any external dependencies we want to keep fortio minimal.

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path"
	"runtime"
	"strings"
	"time"

	"fortio.org/cli"
	"fortio.org/fortio/bincommon"
	"fortio.org/fortio/fgrpc"
	"fortio.org/fortio/fhttp"
	"fortio.org/fortio/fnet"
	"fortio.org/fortio/periodic"
	"fortio.org/fortio/stats"
	"fortio.org/fortio/tcprunner"
	"fortio.org/fortio/udprunner"
	"fortio.org/fortio/ui"
	"fortio.org/fortio/version"
	"fortio.org/log"
	"fortio.org/scli"
)

// -- Start of support for multiple proxies (-P) flags on cmd line.
type proxiesFlagList struct{}

func (f *proxiesFlagList) String() string {
	return ""
}

func (f *proxiesFlagList) Set(value string) error {
	proxies = append(proxies, value)
	return nil
}

// -- End of functions for -P support.

// -- Same for -M.
type httpMultiFlagList struct{}

func (f *httpMultiFlagList) String() string {
	return ""
}

func (f *httpMultiFlagList) Set(value string) error {
	httpMulties = append(httpMulties, value)
	return nil
}

// -- End of -M support.

// fortio's help/args message.
func helpArgsString() string {
	return fmt.Sprintf("target\n%s\n%s\n%s\n%s\n%s\n%s\n%s\n%s\n%s",
		"where command is one of: load (load testing), server (starts ui, rest api,",
		" http-echo, redirect, proxies, tcp-echo, udp-echo and grpc ping servers), ",
		" tcp-echo (only the tcp-echo server), udp-echo (only udp-echo server),",
		" report (report only UI server), redirect (only the redirect server),",
		" proxies (only the -M and -P configured proxies), grpcping (grpc client),",
		" or curl (single URL debug), or nc (single tcp or udp:// connection),",
		" or version (prints the full version and build details).",
		"where target is a url (http load tests) or host:port (grpc health test),",
		" or tcp://host:port (tcp load test), or udp://host:port (udp load test).")
}

// Attention: every flag that is common to http client goes to bincommon/
// for sharing between fortio and fcurl binaries

const (
	disabled = "disabled"
)

var (
	defaults = &periodic.DefaultRunnerOptions
	// Very small default so people just trying with random URLs don't affect the target.
	qpsFlag         = flag.Float64("qps", defaults.QPS, "Queries Per Seconds or 0 for no wait/max qps")
	numThreadsFlag  = flag.Int("c", defaults.NumThreads, "Number of connections/goroutine/threads")
	durationFlag    = flag.Duration("t", defaults.Duration, "How long to run the test or 0 to run until ^C")
	rampFlag        = flag.Duration("ramp", 0, "Ramp/Warm up time from initial to target QPS")
	percentilesFlag = flag.String("p", "50,75,90,99,99.9", "List of pXX to calculate")
	resolutionFlag  = flag.Float64("r", defaults.Resolution, "Resolution of the histogram lowest buckets in seconds")
	offsetFlag      = flag.Duration("offset", defaults.Offset, "Offset of the histogram data")
	goMaxProcsFlag  = flag.Int("gomaxprocs", 0, "Setting for runtime.GOMAXPROCS, <1 doesn't change the default")
	profileFlag     = flag.String("profile", "", "write .cpu and .mem profiles to `file`")
	grpcFlag        = flag.Bool("grpc", false, "Use GRPC (health check by default, add -ping for ping) for load testing")
	echoPortFlag    = flag.String("http-port", "8080",
		"http echo server port. Can be in the form of host:port, ip:port, `port` or /unix/domain/path or \""+disabled+"\".")
	tcpPortFlag = flag.String("tcp-port", "8078",
		"tcp echo server port. Can be in the form of host:port, ip:port, `port` or /unix/domain/path or \""+disabled+"\".")
	udpPortFlag = flag.String("udp-port", "8078",
		"udp echo server port. Can be in the form of host:port, ip:port, `port` or \""+disabled+"\".")
	udpAsyncFlag = flag.Bool("udp-async", false, "if true, udp echo server will use separate go routine to reply")
	grpcPortFlag = flag.String("grpc-port", fnet.DefaultGRPCPort,
		"grpc server port. Can be in the form of host:port, ip:port or `port` or /unix/domain/path or \""+disabled+
			"\" to not start the grpc server.")
	echoDbgPathFlag = flag.String("echo-debug-path", "/debug",
		"http echo server `URI` for debug, empty turns off that part (more secure)")
	jsonFlag = flag.String("json", "",
		"Json output to provided file `path` or '-' for stdout (empty = no json output, unless -a is used)")
	uiPathFlag = flag.String("ui-path", "/fortio/", "http server `URI` for UI, empty turns off that part (more secure)")
	curlFlag   = flag.Bool("curl", false, "Just fetch the content once")
	labelsFlag = flag.String("labels", "",
		"Additional config data/labels to add to the resulting JSON, defaults to target URL and hostname")
	// do not remove the flag for backward compatibility.  Was absolute `path` to the dir containing the static files dir
	// which is now embedded in the binary thanks to that support in golang 1.16.
	_            = flag.String("static-dir", "", "Deprecated/unused `path`.")
	dataDirFlag  = flag.String("data-dir", ".", "`Directory` where JSON results are stored/read")
	proxiesFlags proxiesFlagList
	proxies      = make([]string, 0)
	// -M flag.
	httpMultiFlags httpMultiFlagList
	httpMulties    = make([]string, 0)

	allowInitialErrorsFlag = flag.Bool("allow-initial-errors", false, "Allow and don't abort on initial warmup errors")
	abortOnFlag            = flag.Int("abort-on", 0,
		"Http `code` that if encountered aborts the run. e.g. 503 or -1 for socket errors.")
	autoSaveFlag = flag.Bool("a", false, "Automatically save JSON result with filename based on labels & timestamp")
	redirectFlag = flag.String("redirect-port", "8081", "Redirect all incoming traffic to https URL"+
		" (need ingress to work properly). Can be in the form of host:port, ip:port, `port` or \""+disabled+"\" to disable the feature.")
	exactlyFlag = flag.Int64("n", 0,
		"Run for exactly this number of calls instead of duration. Default (0) is to use duration (-t). "+
			"Default is 1 when used as grpc ping count.")
	syncFlag         = flag.String("sync", "", "index.tsv or s3/gcs bucket xml `URL` to fetch at startup for server modes.")
	syncIntervalFlag = flag.Duration("sync-interval", 0, "Refresh the url every given interval (default, no refresh)")

	baseURLFlag = flag.String("base-url", "",
		"base `URL` used as prefix for data/index.tsv generation. (when empty, the url from the first request is used)")
	newMaxPayloadSizeKb = flag.Int("maxpayloadsizekb", fnet.MaxPayloadSize/fnet.KILOBYTE,
		"MaxPayloadSize is the maximum size of payload to be generated by the EchoHandler size= argument. In `Kbytes`.")

	// GRPC related flags
	// To get most debugging/tracing:
	// GODEBUG="http2debug=2" GRPC_GO_LOG_VERBOSITY_LEVEL=99 GRPC_GO_LOG_SEVERITY_LEVEL=info fortio grpcping -loglevel debug ...
	doHealthFlag   = flag.Bool("health", false, "grpc ping client mode: use health instead of ping")
	doPingLoadFlag = flag.Bool("ping", false, "grpc load test: use ping instead of health")
	healthSvcFlag  = flag.String("healthservice", "", "which service string to pass to health check")
	pingDelayFlag  = flag.Duration("grpc-ping-delay", 0, "grpc ping delay in response")
	streamsFlag    = flag.Int("s", 1, "Number of streams per grpc connection")

	maxStreamsFlag = flag.Uint("grpc-max-streams", 0,
		"MaxConcurrentStreams for the grpc server. Default (0) is to leave the option unset.")
	jitterFlag    = flag.Bool("jitter", false, "set to true to de-synchronize parallel clients' by 10%")
	uniformFlag   = flag.Bool("uniform", false, "set to true to de-synchronize parallel clients' requests uniformly")
	nocatchupFlag = flag.Bool("nocatchup", false,
		"set to exact fixed qps and prevent fortio from trying to catchup when the target fails to keep up temporarily")
	// nc mode flag(s).
	ncDontStopOnCloseFlag = flag.Bool("nc-dont-stop-on-eof", false, "in netcat (nc) mode, don't abort as soon as remote side closes")
	// Mirror origin global setting (should be per destination eventually).
	mirrorOriginFlag = flag.Bool("multi-mirror-origin", true, "Mirror the request url to the target for multi proxies (-M)")
	multiSerialFlag  = flag.Bool("multi-serial-mode", false, "Multi server (-M) requests one at a time instead of parallel mode")
	udpTimeoutFlag   = flag.Duration("udp-timeout", udprunner.UDPTimeOutDefaultValue, "Udp timeout")

	accessLogFileFlag = flag.String("access-log-file", "",
		"file `path` to log all requests to. Maybe have performance impacts")
	accessLogFileFormat = flag.String("access-log-format", "json",
		"`format` for access log. Supported values: [json, influx]")
	calcQPS = flag.Bool("calc-qps", false, "Calculate the qps based on number of requests (-n) and duration (-t)")
)

// serverArgCheck always returns true after checking arguments length.
// so it can be used with isServer = serverArgCheck() below.
func serverArgCheck() bool {
	if len(flag.Args()) != 0 {
		cli.ErrUsage("Error: too many arguments (typo in a flag?)")
	}
	return true
}

func FortioMain(hook bincommon.FortioHook) {
	flag.Var(&proxiesFlags, "P",
		"Tcp proxies to run, e.g -P \"localport1 dest_host1:dest_port1\" -P \"[::1]:0 www.google.com:443\" ...")
	flag.Var(&httpMultiFlags, "M", "Http multi proxy to run, e.g -M \"localport1 baseDestURL1 baseDestURL2\" -M ...")
	bincommon.SharedMain()

	// Use the new [fortio.org/cli] package to handle usage, arguments and flags parsing.
	if cli.ProgramName == "" {
		// fortiotel presets this.
		cli.ProgramName = "Φορτίο"
	}
	cli.ArgsHelp = helpArgsString()
	cli.CommandBeforeFlags = true
	cli.MinArgs = 0   // because `fortio server`s don't take any args
	cli.MaxArgs = 1   // for load, curl etc... subcommands.
	scli.ServerMain() // will Exit if there were arguments/flags errors.

	fnet.ChangeMaxPayloadSize(*newMaxPayloadSizeKb * fnet.KILOBYTE)
	percList, err := stats.ParsePercentiles(*percentilesFlag)
	if err != nil {
		cli.ErrUsage("Unable to extract percentiles from -p: %v", err)
	}
	baseURL := strings.Trim(*baseURLFlag, " \t\n\r/") // remove trailing slash and other whitespace
	sync := strings.TrimSpace(*syncFlag)
	if sync != "" {
		if !ui.Sync(os.Stdout, sync, *dataDirFlag) {
			os.Exit(1)
		}
	}
	isServer := false
	switch cli.Command {
	case "curl":
		fortioLoad(true, nil, hook)
	case "nc":
		fortioNC()
	case "load":
		fortioLoad(*curlFlag, percList, hook)
	case "redirect":
		isServer = serverArgCheck()
		fhttp.RedirectToHTTPS(*redirectFlag)
	case "report":
		isServer = serverArgCheck()
		if *redirectFlag != disabled {
			fhttp.RedirectToHTTPS(*redirectFlag)
		}
		if !ui.Report(baseURL, *echoPortFlag, *dataDirFlag) {
			os.Exit(1) // error already logged
		}
	case "tcp-echo":
		isServer = serverArgCheck()
		fnet.TCPEchoServer("tcp-echo", *tcpPortFlag)
		startProxies()
	case "udp-echo":
		isServer = serverArgCheck()
		fnet.UDPEchoServer("udp-echo", *udpPortFlag, *udpAsyncFlag)
		startProxies()
	case "proxies":
		isServer = serverArgCheck()
		if startProxies() == 0 {
			cli.ErrUsage("Error: fortio proxies command needs at least one -P / -M flag")
		}
	case "server":
		isServer = serverArgCheck()
		if *tcpPortFlag != disabled {
			fnet.TCPEchoServer("tcp-echo", *tcpPortFlag)
		}
		if *udpPortFlag != disabled {
			fnet.UDPEchoServer("udp-echo", *udpPortFlag, *udpAsyncFlag)
		}
		if *grpcPortFlag != disabled {
			fgrpc.PingServer(*grpcPortFlag, *healthSvcFlag, uint32(*maxStreamsFlag), &bincommon.SharedHTTPOptions().TLSOptions)
		}
		if *redirectFlag != disabled {
			fhttp.RedirectToHTTPS(*redirectFlag)
		}
		if *echoPortFlag != disabled {
			if !ui.Serve(hook, baseURL, *echoPortFlag, *echoDbgPathFlag, *uiPathFlag, *dataDirFlag, percList) {
				os.Exit(1) // error already logged
			}
		}
		startProxies()
	case "grpcping":
		grpcClient()
	default:
		cli.ErrUsage("Error: unknown command %q", cli.Command)
	}
	if isServer {
		serverLoop(sync)
	}
}

func serverLoop(sync string) {
	// To get a start time log/timestamp in the logs
	log.Infof("All fortio %s servers started!", version.Long())
	d := *syncIntervalFlag
	if sync != "" && d > 0 {
		log.Infof("Will re-sync data dir every %s", d)
		ticker := time.NewTicker(d)
		defer ticker.Stop()
		for range ticker.C {
			ui.Sync(os.Stdout, sync, *dataDirFlag)
		}
	} else {
		select {}
	}
}

func startProxies() int {
	ctx := context.Background()
	numProxies := 0
	for _, proxy := range proxies {
		s := strings.SplitN(proxy, " ", 2)
		if len(s) != 2 {
			log.Errf("Invalid syntax for proxy \"%s\", should be \"localAddr destHost:destPort\"", proxy)
		}
		fnet.ProxyToDestination(ctx, s[0], s[1])
		numProxies++
	}
	for _, hmulti := range httpMulties {
		s := strings.Split(hmulti, " ")
		if len(s) < 2 {
			log.Errf("Invalid syntax for http multi \"%s\", should be \"localAddr destURL1 destURL2...\"", hmulti)
		}
		mcfg := fhttp.MultiServerConfig{Serial: *multiSerialFlag}
		n := len(s) - 1
		mcfg.Targets = make([]fhttp.TargetConf, n)
		for i := 0; i < n; i++ {
			mcfg.Targets[i].Destination = s[i+1]
			mcfg.Targets[i].MirrorOrigin = *mirrorOriginFlag
		}
		fhttp.MultiServer(s[0], &mcfg)
		numProxies++
	}
	return numProxies
}

func fortioNC() {
	l := len(flag.Args())
	if l != 1 && l != 2 {
		cli.ErrUsage("Error: fortio nc needs a host:port or host port destination")
	}
	d := flag.Args()[0]
	if l == 2 {
		d = d + ":" + flag.Args()[1]
	}
	err := fnet.NetCat(context.Background(), d, os.Stdin, os.Stderr, !*ncDontStopOnCloseFlag /* stop when server closes connection */)
	if err != nil {
		// already logged but exit with error back to shell/caller
		os.Exit(1)
	}
}

//nolint:funlen, gocognit // maybe refactor/shorten later.
func fortioLoad(justCurl bool, percList []float64, hook bincommon.FortioHook) {
	if len(flag.Args()) != 1 {
		cli.ErrUsage("Error: fortio load/curl needs a url or destination")
	}
	httpOpts := bincommon.SharedHTTPOptions()
	if justCurl {
		if hook != nil {
			ro := periodic.RunnerOptions{} // not used, just to call hook for http options for fortiotel curl case
			hook(httpOpts, &ro)
		}
		bincommon.FetchURL(httpOpts)
		return
	}
	url := httpOpts.URL
	prevGoMaxProcs := runtime.GOMAXPROCS(*goMaxProcsFlag)
	out := os.Stderr
	qps := *qpsFlag // TODO possibly use translated <=0 to "max" from results/options normalization in periodic/
	if *calcQPS {
		if *exactlyFlag == 0 || *durationFlag <= 0 {
			cli.ErrUsage("Error: can't use `-calc-qps` without also specifying `-n` and `-t`")
		}
		qps = float64(*exactlyFlag) / durationFlag.Seconds()
		log.LogVf("Calculated QPS to do %d request in %v: %f", *exactlyFlag, *durationFlag, qps)
	}
	_, _ = fmt.Fprintf(out, "Fortio %s running at %g queries per second, %d->%d procs",
		version.Short(), qps, prevGoMaxProcs, runtime.GOMAXPROCS(0))
	if *exactlyFlag > 0 {
		_, _ = fmt.Fprintf(out, ", for %d calls: %s\n", *exactlyFlag, url)
	} else {
		if *durationFlag <= 0 {
			// Infinite mode is determined by having a negative duration value
			*durationFlag = -1
			_, _ = fmt.Fprintf(out, ", until interrupted: %s\n", url)
		} else {
			_, _ = fmt.Fprintf(out, ", for %v: %s\n", *durationFlag, url)
		}
	}
	if qps <= 0 {
		qps = -1 // 0==unitialized struct == default duration, -1 (0 for flag) is max
	}
	labels := *labelsFlag
	if labels == "" {
		hname, _ := os.Hostname()
		shortURL := url
		for _, p := range []string{"https://", "http://"} {
			if strings.HasPrefix(url, p) {
				shortURL = url[len(p):]
				break
			}
		}
		labels = shortURL + " , " + strings.SplitN(hname, ".", 2)[0]
		log.LogVf("Generated Labels: %s", labels)
	}
	ro := periodic.RunnerOptions{
		QPS:         qps,
		Duration:    *durationFlag,
		Ramp:        *rampFlag,
		NumThreads:  *numThreadsFlag,
		Percentiles: percList,
		Resolution:  *resolutionFlag,
		Out:         out,
		Labels:      labels,
		Exactly:     *exactlyFlag,
		Jitter:      *jitterFlag,
		Uniform:     *uniformFlag,
		RunID:       *bincommon.RunIDFlag,
		Offset:      *offsetFlag,
		NoCatchUp:   *nocatchupFlag,
	}
	err := ro.AddAccessLogger(*accessLogFileFlag, *accessLogFileFormat)
	if err != nil {
		// Error already logged.
		os.Exit(1)
	}
	var res periodic.HasRunnerResult
	if hook != nil {
		hook(httpOpts, &ro)
	}
	if *grpcFlag {
		o := fgrpc.GRPCRunnerOptions{
			RunnerOptions:      ro,
			Destination:        url,
			Service:            *healthSvcFlag,
			Streams:            *streamsFlag,
			AllowInitialErrors: *allowInitialErrorsFlag,
			Payload:            httpOpts.PayloadUTF8(),
			Delay:              *pingDelayFlag,
			UsePing:            *doPingLoadFlag,
			Metadata:           httpHeader2grpcMetadata(httpOpts.AllHeaders()),
		}
		o.TLSOptions = httpOpts.TLSOptions
		res, err = fgrpc.RunGRPCTest(&o)
	} else if strings.HasPrefix(url, tcprunner.TCPURLPrefix) {
		o := tcprunner.RunnerOptions{
			RunnerOptions: ro,
		}
		o.ReqTimeout = httpOpts.HTTPReqTimeOut
		o.Destination = url
		o.Payload = httpOpts.Payload
		res, err = tcprunner.RunTCPTest(&o)
	} else if strings.HasPrefix(url, udprunner.UDPURLPrefix) {
		o := udprunner.RunnerOptions{
			RunnerOptions: ro,
		}
		o.ReqTimeout = *udpTimeoutFlag
		o.Destination = url
		o.Payload = httpOpts.Payload
		res, err = udprunner.RunUDPTest(&o)
	} else {
		o := fhttp.HTTPRunnerOptions{
			HTTPOptions:        *httpOpts,
			RunnerOptions:      ro,
			Profiler:           *profileFlag,
			AllowInitialErrors: *allowInitialErrorsFlag,
			AbortOn:            *abortOnFlag,
		}
		res, err = fhttp.RunHTTPTest(&o)
	}
	if err != nil {
		_, _ = fmt.Fprintf(out, "Aborting because of %v\n", err)
		os.Exit(1)
	}
	rr := res.Result()
	warmup := *numThreadsFlag
	if ro.Exactly > 0 {
		warmup = 0
	}
	_, _ = fmt.Fprintf(out, "All done %d calls (plus %d warmup) %.3f ms avg, %.1f qps\n",
		rr.DurationHistogram.Count,
		warmup,
		1000.*rr.DurationHistogram.Avg,
		rr.ActualQPS)
	jsonFileName := *jsonFlag
	if *autoSaveFlag || len(jsonFileName) > 0 { //nolint:nestif // but probably should breakup this function
		var j []byte
		j, err = json.MarshalIndent(res, "", "  ")
		if err != nil {
			log.Fatalf("Unable to json serialize result: %v", err)
		}
		var f *os.File
		if jsonFileName == "-" {
			f = os.Stdout
			jsonFileName = "stdout"
		} else {
			if len(jsonFileName) == 0 {
				jsonFileName = path.Join(*dataDirFlag, rr.ID+".json")
			}
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
		_, _ = fmt.Fprintf(out, "Successfully wrote %d bytes of Json data to %s\n", n, jsonFileName)
	}
}

func grpcClient() {
	if len(flag.Args()) != 1 {
		cli.ErrUsage("Error: fortio grpcping needs host argument in the form of host, host:port or ip:port")
	}
	host := flag.Arg(0)
	count := int(*exactlyFlag)
	if count <= 0 {
		count = 1
	}
	httpOpts := bincommon.SharedHTTPOptions()
	md := httpHeader2grpcMetadata(httpOpts.AllHeaders())
	if *doHealthFlag {
		status, err := fgrpc.GrpcHealthCheck(host, *healthSvcFlag, count, &httpOpts.TLSOptions, md)
		if err != nil {
			// already logged
			os.Exit(1)
		}
		if (*status)["SERVING"] != int64(count) {
			log.Errf("Unexpected SERVING count %d vs %d", (*status)["SERVING"], count)
			os.Exit(1)
		}
		return
	}
	_, err := fgrpc.PingClientCall(host, count, httpOpts.PayloadUTF8(), *pingDelayFlag, &httpOpts.TLSOptions, md)
	if err != nil {
		// already logged
		os.Exit(1)
	}
}

// httpHeader2grpcMetadata covert md's key to lowercase and filter invalid key.
func httpHeader2grpcMetadata(headers map[string][]string) map[string][]string {
	ret := make(map[string][]string)
	for k, v := range headers {
		k = strings.ToLower(k)
		switch k {
		case "content-length", "content-type":
			log.LogVf("Skipping setting metadata %s:%v", k, v)
			// shouldn't set for grpc
			continue
		}
		ret[k] = v
		log.Debugf("Setting metadata %s:%v", k, v)
	}
	return ret
}
