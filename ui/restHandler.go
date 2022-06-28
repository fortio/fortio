package ui // import "fortio.org/fortio/ui"

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"fortio.org/fortio/bincommon"
	"fortio.org/fortio/fgrpc"
	"fortio.org/fortio/fhttp"
	"fortio.org/fortio/log"
	"fortio.org/fortio/periodic"
	"fortio.org/fortio/stats"
	"fortio.org/fortio/tcprunner"
	"fortio.org/fortio/udprunner"
)

// ErrorReply is returned on errors.
type ErrorReply struct {
	Error     string
	Exception error
}

// Error writes serialized ErrorReply to the writer.
func Error(w http.ResponseWriter, msg ErrorReply) {
	if w == nil {
		// async mode, nothing to do
		return
	}
	w.WriteHeader(http.StatusBadRequest)
	b, _ := json.Marshal(msg)
	_, _ = w.Write(b)
}

// GetConfigAtPath deserializes the bytes as JSON and
// extracts the map at the given path (only supports simple expression:
// . is all the json
// .foo.bar.blah will extract that part of the tree.
func GetConfigAtPath(path string, data []byte) (map[string]interface{}, error) {
	// that's what Unmarshal does anyway if you pass interface{} var, skips a cast even for dynamic/unknown json
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return getConfigAtPath(path, m)
}

// recurse on the requested path.
func getConfigAtPath(path string, m map[string]interface{}) (map[string]interface{}, error) {
	path = strings.TrimLeft(path, ".")
	if path == "" {
		return m, nil
	}
	parts := strings.SplitN(path, ".", 2)
	log.Debugf("split got us %v", parts)
	first := parts[0]
	rest := ""
	if len(parts) == 2 {
		rest = parts[1]
	}
	nm, found := m[first]
	if !found {
		return nil, fmt.Errorf("%q not found in json", first)
	}
	mm, ok := nm.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("%q path is not a map", first)
	}
	return getConfigAtPath(rest, mm)
}

// FormValue gets the value from the query arguments/url parameter or from the
// provided map (json data).
func FormValue(r *http.Request, json map[string]interface{}, key string) string {
	// query args have priority
	res := r.FormValue(key)
	if res != "" {
		log.Debugf("key %q in query args so using that value %q", key, res)
		return res
	}
	res2, found := json[key]
	if !found {
		log.Debugf("key %q not in json map", key)
		return ""
	}
	res, ok := res2.(string)
	if !ok {
		log.Warnf("%q is %+v / not a string, can't be used", key, res2)
		return ""
	}
	log.LogVf("Getting %q from json: %q", key, res)
	return res
}

// RESTRunHandler is api version of UI submit handler.
func RESTRunHandler(w http.ResponseWriter, r *http.Request) { // nolint: funlen
	fhttp.LogRequest(r, "REST Run Api call")
	w.Header().Set("Content-Type", "application/json")
	data, err := ioutil.ReadAll(r.Body) // must be done before calling FormValue
	if err != nil {
		log.Errf("Error reading %v", err)
		Error(w, ErrorReply{"body read error", err})
		return
	}
	log.Infof("REST body: %s", fhttp.DebugSummary(data, 250))
	jsonPath := r.FormValue("jsonPath")
	var jd map[string]interface{}
	if len(data) > 0 {
		// Json input and deserialize options from that path, eg. for flagger:
		// jsonPath=metadata
		jd, err = GetConfigAtPath(jsonPath, data)
		if err != nil {
			log.Errf("Error deserializing %v", err)
			Error(w, ErrorReply{"body json deserialization error: " + err.Error(), err})
			return
		}
		log.Infof("Body: %+v", jd)
	}
	url := FormValue(r, jd, "url")
	runner := FormValue(r, jd, "runner")
	if runner == "" {
		runner = "http"
	}
	log.Infof("Starting API run %s load request from %v for %s", runner, r.RemoteAddr, url)
	async := (FormValue(r, jd, "async") == "on")
	payload := FormValue(r, jd, "payload")
	labels := FormValue(r, jd, "labels")
	resolution, _ := strconv.ParseFloat(FormValue(r, jd, "r"), 64)
	percList, _ := stats.ParsePercentiles(FormValue(r, jd, "p"))
	qps, _ := strconv.ParseFloat(FormValue(r, jd, "qps"), 64)
	durStr := FormValue(r, jd, "t")
	jitter := (FormValue(r, jd, "jitter") == "on")
	uniform := (FormValue(r, jd, "uniform") == "on")
	nocatchup := (FormValue(r, jd, "nocatchup") == "on")
	stdClient := (FormValue(r, jd, "stdclient") == "on")
	sequentialWarmup := (FormValue(r, jd, "sequential-warmup") == "on")
	httpsInsecure := (FormValue(r, jd, "https-insecure") == "on")
	resolve := FormValue(r, jd, "resolve")
	timeoutStr := strings.TrimSpace(FormValue(r, jd, "timeout"))
	timeout, _ := time.ParseDuration(timeoutStr) // will be 0 if empty, which is handled by runner and opts
	var dur time.Duration
	if durStr == "on" {
		dur = -1
	} else {
		var err error
		dur, err = time.ParseDuration(durStr)
		if err != nil {
			log.Errf("Error parsing duration '%s': %v", durStr, err)
		}
	}
	c, _ := strconv.Atoi(FormValue(r, jd, "c"))
	out := io.Writer(os.Stderr)
	if len(percList) == 0 && !strings.Contains(r.URL.RawQuery, "p=") {
		percList = defaultPercentileList
	}
	n, _ := strconv.ParseInt(FormValue(r, jd, "n"), 10, 64)
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
		Uniform:     uniform,
		NoCatchUp:   nocatchup,
	}
	ro.Normalize()
	uiRunMapMutex.Lock()
	id++ // start at 1 as 0 means interrupt all
	runid := id
	runs[runid] = &ro
	uiRunMapMutex.Unlock()
	ro.RunID = runid
	log.Infof("New run id %d", runid)
	httpopts := &fhttp.HTTPOptions{}
	httpopts.HTTPReqTimeOut = timeout // to be normalized in init 0 replaced by default value
	httpopts = httpopts.Init(url)
	httpopts.ResetHeaders()
	httpopts.DisableFastClient = stdClient
	httpopts.SequentialWarmup = sequentialWarmup
	httpopts.Insecure = httpsInsecure
	httpopts.Resolve = resolve
	// Set the connection reuse range.
	err = bincommon.ConnectionReuseRange.
		WithValidator(bincommon.ConnectionReuseRangeValidator(httpopts)).
		Set(FormValue(r, jd, "connection-reuse-range"))
	if err != nil {
		log.Errf("Fail to validate connection reuse range flag, err: %v", err)
	}

	if len(payload) > 0 {
		httpopts.Payload = []byte(payload)
	}
	jsonHeaders, found := jd["headers"]
	for found { // really an if, but using while to break out without else below
		res, ok := jsonHeaders.([]interface{})
		if !ok {
			log.Warnf("Json Headers is %T %v / not an array, can't be used", jsonHeaders, jsonHeaders)
			break
		}
		for _, header := range res {
			log.LogVf("adding json header %T: %v", header, header)
			hStr, ok := header.(string)
			if !ok {
				log.Errf("Json headers must be an array of strings (got %T: %v)", header, header)
				continue
			}
			if err := httpopts.AddAndValidateExtraHeader(hStr); err != nil {
				log.Errf("Error adding custom json headers: %v", err)
			}
		}
		break
	}
	for _, header := range r.Form["H"] {
		if len(header) == 0 {
			continue
		}
		log.LogVf("adding query arg header %v", header)
		err := httpopts.AddAndValidateExtraHeader(header)
		if err != nil {
			log.Errf("Error adding custom query arg headers: %v", err)
		}
	}
	fhttp.OnBehalfOf(httpopts, r)
	if async {
		w.Write([]byte(fmt.Sprintf("{\"started\": %d}", runid)))
		go Run(nil, r, jd, runner, url, ro, httpopts)
		return
	}
	Run(w, r, jd, runner, url, ro, httpopts)
}

// Run executes the run (can be called async or not, writer is nil for async mode).
func Run(w http.ResponseWriter, r *http.Request, jd map[string]interface{},
	runner, url string, ro periodic.RunnerOptions, httpopts *fhttp.HTTPOptions,
) {
	//	go func() {
	var res periodic.HasRunnerResult
	var err error
	if runner == modegrpc { // nolint: nestif
		grpcSecure := (FormValue(r, jd, "grpc-secure") == "on")
		grpcPing := (FormValue(r, jd, "ping") == "on")
		grpcPingDelay, _ := time.ParseDuration(FormValue(r, jd, "grpc-ping-delay"))
		o := fgrpc.GRPCRunnerOptions{
			RunnerOptions: ro,
			Destination:   url,
			UsePing:       grpcPing,
			Delay:         grpcPingDelay,
		}
		o.TLSOptions = httpopts.TLSOptions
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
		o.ReqTimeout = httpopts.HTTPReqTimeOut
		o.Destination = url
		o.Payload = httpopts.Payload
		res, err = tcprunner.RunTCPTest(&o)
	} else if strings.HasPrefix(url, udprunner.UDPURLPrefix) {
		// TODO: copy pasta from fortio_main
		o := udprunner.RunnerOptions{
			RunnerOptions: ro,
		}
		o.ReqTimeout = httpopts.HTTPReqTimeOut
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
	uiRunMapMutex.Lock()
	delete(runs, ro.RunID)
	uiRunMapMutex.Unlock()
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
	doSave := (FormValue(r, jd, "save") == "on")
	if doSave {
		SaveJSON(id, json)
	}
	if w == nil {
		// async, no result to output
		return
	}
	_, err = w.Write(json)
	if err != nil {
		log.Errf("Unable to write json output for %v: %v", r.RemoteAddr, err)
	}
}

// RESTStatusHandler will print the state of the runs.
func RESTStatusHandler(w http.ResponseWriter, r *http.Request) {
	fhttp.LogRequest(r, "REST Status Api call")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	w.Write([]byte("{\"error\":\"status not yet implemented\"}"))
}

// RESTStopHandler is the api to stop a given run by runid or all the runs if unspecified/0.
func RESTStopHandler(w http.ResponseWriter, r *http.Request) {
	fhttp.LogRequest(r, "REST Stop Api call")
	w.Header().Set("Content-Type", "application/json")
	runid, _ := strconv.ParseInt(r.FormValue("runid"), 10, 64)
	i := StopByRunID(runid)
	w.Write([]byte(fmt.Sprintf("{\"stopped\": %d}", i)))
}

// StopByRunID stops all the runs if passed 0 or the runid provided.
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
