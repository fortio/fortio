package ui // import "fortio.org/fortio/ui"

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"fortio.org/fortio/fgrpc"
	"fortio.org/fortio/fhttp"
	"fortio.org/fortio/log"
	"fortio.org/fortio/periodic"
	"fortio.org/fortio/stats"
	"fortio.org/fortio/tcprunner"
	"fortio.org/fortio/udprunner"
)

type ErrorReply struct {
	Error     string
	Exception error
}

func Error(w http.ResponseWriter, msg ErrorReply) {
	w.WriteHeader(http.StatusBadRequest)
	b, _ := json.Marshal(msg)
	_, _ = w.Write(b)
}

// RESTHandler is api version of UI submit handler
func RESTRunHandler(w http.ResponseWriter, r *http.Request) {
	fhttp.LogRequest(r, "REST Run Api call")
	DoSave := (r.FormValue("save") == "on")
	url := r.FormValue("url")
	runner := r.FormValue("runner")
	if runner == "" {
		runner = "http"
	}
	log.Infof("Starting API run %s load request from %v for %s", runner, r.RemoteAddr, url)
	async := (r.FormValue("async") == "on")
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
		if err != nil {
			log.Errf("Error parsing duration '%s': %v", durStr, err)
		}
	}
	c, _ := strconv.Atoi(r.FormValue("c"))
	out := io.Writer(os.Stderr)
	if len(percList) == 0 && !strings.Contains(r.URL.RawQuery, "p=") {
		percList = defaultPercentileList
	}
	n, _ := strconv.ParseInt(r.FormValue("n"), 10, 64)
	if strings.TrimSpace(url) == "" {
		Error(w, ErrorReply{"URL is required", nil})
		return
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
	ro.Normalize()
	uiRunMapMutex.Lock()
	id++ // start at 1 as 0 means interrupt all
	runid := id
	runs[runid] = &ro
	uiRunMapMutex.Unlock()
	log.Infof("New run id %d", runid)
	httpopts := &fhttp.HTTPOptions{}
	httpopts.HTTPReqTimeOut = timeout // to be normalized in init 0 replaced by default value
	httpopts = httpopts.Init(url)
	httpopts.ResetHeaders()
	httpopts.DisableFastClient = stdClient
	httpopts.Insecure = httpsInsecure
	httpopts.Resolve = resolve
	if len(payload) > 0 {
		httpopts.Payload = []byte(payload)
	}
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
	if async {
		w.Write([]byte(fmt.Sprintf("{\"started\": %d}", runid)))
		// detach?
	}
	//	go func() {
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
		Error(w, ErrorReply{"Aborting because of error", err})
		return
	}
	json, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		log.Fatalf("Unable to json serialize result: %v", err)
	}
	id := res.Result().ID()
	if DoSave {
		SaveJSON(id, json)
	}
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(json)
	if err != nil {
		log.Errf("Unable to write json output for %v: %v", r.RemoteAddr, err)
	}
	uiRunMapMutex.Lock()
	delete(runs, runid)
	uiRunMapMutex.Unlock()
	//	}()
}

func RESTStatusHandler(w http.ResponseWriter, r *http.Request) {
	fhttp.LogRequest(r, "REST Status Api call")
	w.WriteHeader(http.StatusServiceUnavailable)
	w.Write([]byte("{\"error\":\"status not yet implemented\"}"))
}

func RESTStopHandler(w http.ResponseWriter, r *http.Request) {
	fhttp.LogRequest(r, "REST Stop Api call")
	i := StopByRunID(0) // TODO: get from input
	w.Write([]byte(fmt.Sprintf("{\"stopped\": %d}", i)))
}

func StopByRunID(runid int64) int {
	uiRunMapMutex.Lock()
	if runid <= 0 { // Stop all
		i := 0
		for k, v := range runs {
			v.Abort()
			delete(runs, k)
			i++
		}
		uiRunMapMutex.Unlock()
		log.Infof("Interrupted all %d runs", i)
		return i
	}
	// else: Stop one
	v, found := runs[runid]
	if found {
		delete(runs, runid)
		uiRunMapMutex.Unlock()
		v.Abort()
		log.Infof("Interrupted run id %d", runid)
		return 1
	}
	log.Infof("Runid %d not found to interrupt", runid)
	uiRunMapMutex.Unlock()
	return 0
}
