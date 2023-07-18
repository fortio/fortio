// Copyright 2022 Fortio Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Remote API to trigger load tests package (REST API).
package rapi // import "fortio.org/fortio/rapi"

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"fortio.org/fortio/bincommon"
	"fortio.org/fortio/fgrpc"
	"fortio.org/fortio/fhttp"
	"fortio.org/fortio/fnet"
	"fortio.org/fortio/jrpc"
	"fortio.org/fortio/periodic"
	"fortio.org/fortio/stats"
	"fortio.org/fortio/tcprunner"
	"fortio.org/fortio/udprunner"
	"fortio.org/log"
)

const (
	RestRunURI    = "rest/run"
	RestStatusURI = "rest/status"
	RestStopURI   = "rest/stop"
	RestDNS       = "rest/dns"
	ModeGRPC      = "grpc"
)

type StateEnum int

const (
	StateUnknown StateEnum = iota
	StatePending
	StateRunning
	StateStopping
	StateStopped
)

func (se StateEnum) String() string {
	switch se {
	case StateUnknown:
		return "unknown"
	case StatePending:
		return "pending"
	case StateRunning:
		return "running"
	case StateStopping:
		return "stopping"
	case StateStopped:
		return "stopped"
	}
	panic("unknown state")
}

type Status struct {
	RunID         int64
	State         StateEnum
	RunnerOptions *periodic.RunnerOptions
	aborter       *periodic.Aborter
}

type StatusMap map[int64]*Status

var (
	uiRunMapMutex = &sync.Mutex{}
	id            int64
	runs          = make(StatusMap)
	// Directory where results are written to/read from.
	dataDir string
	// Default percentiles when not otherwise specified.
	DefaultPercentileList []float64
	// Hook to install OTEL or other options.
	hook bincommon.FortioHook
)

// AsyncReply is returned when async=on is passed.
type AsyncReply struct {
	jrpc.ServerReply
	RunID int64
	Count int
	// Object id to retrieve results (only usable if save=on).
	// Also returned when using stop as long as exactly 1 run is stopped.
	ResultID string
	// Result url, constructed from the ResultID and the incoming request URL, if available.
	ResultURL string
}

type StatusReply struct {
	jrpc.ServerReply
	Statuses StatusMap
}

type DNSReply struct {
	jrpc.ServerReply
	Name string
	IPv4 []string
	IPv6 []string
}

// Error writes serialized ServerReply marked as error, to the writer.
func Error(w http.ResponseWriter, msg string, err error) {
	if w == nil {
		// async mode: nothing to reply
		return
	}
	_ = jrpc.ReplyError(w, msg, err)
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
	if json == nil { // When called from uihandler we don't have a json map.
		log.Debugf("no json data so returning empty string for key %q", key)
		return ""
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
// TODO: refactor common option/args/flag parsing between uihandler.go and this.
func RESTRunHandler(w http.ResponseWriter, r *http.Request) { //nolint:funlen
	log.LogRequest(r, "REST Run call")
	w.Header().Set("Content-Type", "application/json")
	data, err := io.ReadAll(r.Body) // must be done before calling FormValue
	if err != nil {
		log.Errf("Error reading %v", err)
		Error(w, "body read error", err)
		return
	}
	log.Infof("REST body: %s", fhttp.DebugSummary(data, 250))
	jsonPath := r.FormValue("jsonPath")
	var jd map[string]interface{}
	if len(data) > 0 {
		// Json input and deserialize options from that path, eg. for flagger:
		// jsonPath=.metadata
		jd, err = GetConfigAtPath(jsonPath, data)
		if err != nil {
			log.Errf("Error deserializing %v", err)
			Error(w, "body json deserialization error", err)
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
	methodOverride := FormValue(r, jd, "X")
	labels := FormValue(r, jd, "labels")
	resolution, _ := strconv.ParseFloat(FormValue(r, jd, "r"), 64)
	percList, _ := stats.ParsePercentiles(FormValue(r, jd, "p"))
	qps, _ := strconv.ParseFloat(FormValue(r, jd, "qps"), 64)
	durStr := FormValue(r, jd, "t")
	jitter := (FormValue(r, jd, "jitter") == "on")
	uniform := (FormValue(r, jd, "uniform") == "on")
	logErrors := (FormValue(r, jd, "log-errors") == "on")
	nocatchup := (FormValue(r, jd, "nocatchup") == "on")
	stdClient := (FormValue(r, jd, "stdclient") == "on")
	h2 := (FormValue(r, jd, "h2") == "on")
	sequentialWarmup := (FormValue(r, jd, "sequential-warmup") == "on")
	httpsInsecure := (FormValue(r, jd, "https-insecure") == "on")
	resolve := FormValue(r, jd, "resolve")
	timeoutStr := strings.TrimSpace(FormValue(r, jd, "timeout"))
	timeout, _ := time.ParseDuration(timeoutStr) // will be 0 if empty, which is handled by runner and opts
	var dur time.Duration
	if durStr == "on" {
		dur = -1
	} else if durStr != "" {
		dur, err = time.ParseDuration(durStr)
		if err != nil {
			log.Errf("Error parsing duration '%s': %v", durStr, err)
			Error(w, "parsing duration", err)
			return
		}
	}
	c, _ := strconv.Atoi(FormValue(r, jd, "c"))
	out := io.Writer(os.Stderr)
	if len(percList) == 0 && !strings.Contains(r.URL.RawQuery, "p=") {
		percList = DefaultPercentileList
	}
	n, _ := strconv.ParseInt(FormValue(r, jd, "n"), 10, 64)
	if strings.TrimSpace(url) == "" {
		Error(w, "URL is required", nil)
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
	runid := NextRunID()
	ro.RunID = runid
	log.Infof("New run id %d", runid)
	httpopts := &fhttp.HTTPOptions{}
	httpopts.HTTPReqTimeOut = timeout // to be normalized in init, 0 is replaced by default value (for all runners)
	// We don't call Init because this could be a tcp:// or udp:// url. Was httpopts = httpopts.Init(url) - fixes #651
	httpopts.URL = url
	httpopts.DisableFastClient = stdClient
	httpopts.SequentialWarmup = sequentialWarmup
	httpopts.Insecure = httpsInsecure
	httpopts.Resolve = resolve
	httpopts.H2 = h2
	httpopts.LogErrors = logErrors
	httpopts.MethodOverride = methodOverride
	// Set the connection reuse range.
	err = bincommon.ConnectionReuseRange.
		WithValidator(bincommon.ConnectionReuseRangeValidator(httpopts)).
		Set(FormValue(r, jd, "connection-reuse"))
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
	// Needed to reply the id, will be reused in Normalize() later as already set
	// but we also do it early even for sync case so that the id is available to save JSON
	// in case of init error.
	ro.GenID()
	if async {
		reply := AsyncReply{RunID: runid, Count: 1, ResultID: ro.ID, ResultURL: ID2URL(r, ro.ID)}
		reply.Message = "started" //nolint:goconst
		err := jrpc.ReplyOk(w, &reply)
		if err != nil {
			log.Errf("Error replying to start: %v", err)
		}
		//nolint:errcheck,contextcheck // all cases handled inside for rapi callers. async code with our own aborter
		// returned values are for the ui/uihandler.go caller.
		go Run(nil, r, jd, runner, url, &ro, httpopts, false)
		return
	}
	//nolint:errcheck,contextcheck // all cases handled inside for rapi callers. aborter handles context.
	Run(w, r, jd, runner, url, &ro, httpopts, false)
}

// Run executes the run (can be called async or not, writer is nil for async mode).
// Api is a bit awkward to be compatible with both this new now main REST code but
// also the old one in ui/uihandler.go.
func Run(w http.ResponseWriter, r *http.Request, jd map[string]interface{},
	runner, url string, ro *periodic.RunnerOptions, httpopts *fhttp.HTTPOptions, htmlMode bool,
) (periodic.HasRunnerResult, string, []byte, error) {
	var res periodic.HasRunnerResult
	var err error
	var aborter *periodic.Aborter
	if hook != nil {
		hook(httpopts, ro)
	}
	if runner == ModeGRPC { //nolint:nestif
		grpcSecure := (FormValue(r, jd, "grpc-secure") == "on")
		grpcPing := (FormValue(r, jd, "ping") == "on")
		grpcPingDelay, _ := time.ParseDuration(FormValue(r, jd, "grpc-ping-delay"))
		o := fgrpc.GRPCRunnerOptions{
			RunnerOptions: *ro,
			Destination:   url,
			UsePing:       grpcPing,
			Delay:         grpcPingDelay,
			Service:       FormValue(r, jd, "healthservice"),
		}
		o.TLSOptions = httpopts.TLSOptions
		if grpcSecure {
			o.Destination = fhttp.AddHTTPS(url)
		}
		aborter = UpdateRun(&o.RunnerOptions)
		// TODO: ReqTimeout: timeout
		res, err = fgrpc.RunGRPCTest(&o)
	} else if strings.HasPrefix(url, tcprunner.TCPURLPrefix) {
		// TODO: copy pasta from fortio_main
		o := tcprunner.RunnerOptions{
			RunnerOptions: *ro,
		}
		o.ReqTimeout = httpopts.HTTPReqTimeOut
		o.Destination = url
		o.Payload = httpopts.Payload
		aborter = UpdateRun(&o.RunnerOptions)
		res, err = tcprunner.RunTCPTest(&o)
	} else if strings.HasPrefix(url, udprunner.UDPURLPrefix) {
		// TODO: copy pasta from fortio_main
		o := udprunner.RunnerOptions{
			RunnerOptions: *ro,
		}
		o.ReqTimeout = httpopts.HTTPReqTimeOut
		o.Destination = url
		o.Payload = httpopts.Payload
		aborter = UpdateRun(&o.RunnerOptions)
		res, err = udprunner.RunUDPTest(&o)
	} else {
		o := fhttp.HTTPRunnerOptions{
			HTTPOptions:        *httpopts,
			RunnerOptions:      *ro,
			AllowInitialErrors: true,
		}
		aborter = UpdateRun(&(o.RunnerOptions))
		res, err = fhttp.RunHTTPTest(&o)
	}
	defer RemoveRun(ro.RunID)
	defer func() {
		log.LogVf("REST run %d really done - before channel write", ro.RunID)
		aborter.StartChan <- false
		log.LogVf("REST run %d really done - after channel write", ro.RunID)
	}()
	savedAs := ""
	jsonData, jerr := json.MarshalIndent(res, "", "  ")
	if jerr != nil {
		log.Fatalf("Unable to json serialize result: %v", jerr) //nolint:gocritic // gocritic doesn't know fortio's log.Fatalf does pani
	}
	jsonStr := string(jsonData)
	log.LogVf("Serialized to %s", jsonStr)
	id := res.Result().ID
	doSave := (FormValue(r, jd, "save") == "on")
	if doSave && id != "" {
		savedAs = SaveJSON(id, jsonData)
	}
	if err != nil {
		log.Errf("Init error for %s mode with url %s and options %+v : %v", runner, url, ro, err)
		if !htmlMode {
			Error(w, "Aborting because of error", err)
		}
		return res, "", nil, err
	}
	if w == nil {
		// async or html but nil w (no json output): no result to output
		return res, savedAs, jsonData, nil
	}
	if htmlMode {
		// Already set in api mode but not in html mode
		w.Header().Set("Content-Type", "application/json")
	}
	_, err = w.Write(jsonData)
	if err != nil {
		log.Errf("Unable to write json output for %v: %v", r.RemoteAddr, err)
	}
	return res, savedAs, jsonData, nil
}

// RESTStatusHandler will print the state of the runs.
func RESTStatusHandler(w http.ResponseWriter, r *http.Request) {
	log.LogRequest(r, "REST Status call")
	runid, _ := strconv.ParseInt(r.FormValue("runid"), 10, 64)
	statusReply := StatusReply{}
	if runid != 0 {
		ro := GetRun(runid)
		if ro != nil {
			statusReply.Statuses = StatusMap{runid: ro}
		}
	} else {
		statusReply.Statuses = GetAllRuns()
	}
	err := jrpc.ReplyOk(w, &statusReply)
	if err != nil {
		log.Errf("Error replying to status: %v", err)
	}
}

// RESTStopHandler is the api to stop a given run by runid or all the runs if unspecified/0.
func RESTStopHandler(w http.ResponseWriter, r *http.Request) {
	log.LogRequest(r, "REST Stop call")
	runid, _ := strconv.ParseInt(r.FormValue("runid"), 10, 64)
	waitStr := strings.ToLower(r.FormValue("wait"))
	wait := (waitStr != "" && waitStr != "off" && waitStr != "false")
	i, rid := StopByRunID(runid, wait)
	reply := AsyncReply{RunID: runid, Count: i, ResultID: rid, ResultURL: ID2URL(r, rid)}
	if wait && i == 1 {
		reply.Message = StateStopped.String()
	} else {
		reply.Message = StateStopping.String()
	}
	err := jrpc.ReplyOk(w, &reply)
	if err != nil {
		log.Errf("Error replying: %v", err)
	}
}

// StopByRunID stops all the runs if passed 0 or the runid provided.
// if wait is true, waits for the run to actually end (single only).
func StopByRunID(runid int64, wait bool) (int, string) {
	uiRunMapMutex.Lock()
	rid := ""
	if runid <= 0 { // Stop all
		i := 0
		for _, v := range runs {
			if v.State != StateRunning {
				continue
			}
			v.State = StateStopping // We'll let Run() do the actual removal
			v.aborter.Abort(wait)
			rid = v.RunnerOptions.ID
			i++
		}
		uiRunMapMutex.Unlock()
		log.Infof("Interrupted all %d runs", i)
		if i > 1 {
			// if we stopped more than 1 don't mislead that we have the file IDs
			rid = ""
		}
		return i, rid
	}
	// else: Stop one
	v, found := runs[runid]
	if !found {
		uiRunMapMutex.Unlock()
		log.Infof("Runid %d not found to interrupt", runid)
		return 0, rid
	}
	if v.State != StateRunning {
		uiRunMapMutex.Unlock()
		log.Infof("Runid %d is not running it's %s", runid, v.State.String())
		return 0, rid
	}
	rid = v.RunnerOptions.ID
	v.State = StateStopping
	// We leave it in the map and let the original Run() remove itself once it actually ends
	uiRunMapMutex.Unlock()
	v.aborter.Abort(wait)
	if wait {
		log.LogVf("REST stop, wait requested, reading additional channel signal")
		<-v.aborter.StartChan
		log.LogVf("REST stop, received all done signal")
	}
	log.LogVf("Returning from Abort %d call with wait %v", runid, wait)
	return 1, rid
}

func RemoveRun(id int64) {
	uiRunMapMutex.Lock()
	// If we kept the entries we'd set it to StateStopped
	delete(runs, id)
	uiRunMapMutex.Unlock()
	log.LogVf("REST Removed run %d", id)
}

func RESTDNSHandler(w http.ResponseWriter, r *http.Request) {
	log.LogRequest(r, "REST DNS call")
	name := r.FormValue("name")
	ips, err := fnet.ResolveAll(r.Context(), name, "ip")
	if err != nil {
		err = jrpc.ReplyError(w, "dns failed", err)
		if err != nil {
			log.Errf("Error replying: %v", err)
		}
		return
	}
	reply := DNSReply{Name: name, IPv4: make([]string, 0), IPv6: make([]string, 0)}
	for _, ip := range ips {
		if ip.To4() == nil {
			reply.IPv6 = append(reply.IPv6, ip.String())
		} else {
			reply.IPv4 = append(reply.IPv4, ip.String())
		}
	}
	err = jrpc.ReplyOk(w, &reply)
	if err != nil {
		log.Errf("Error replying: %v", err)
	}
}

// AddHandlers adds the REST Api handlers for run, status and stop.
// uiPath must end with a /.
func AddHandlers(ahook bincommon.FortioHook, mux *http.ServeMux, baseurl, uiPath, datadir string) {
	hook = ahook
	AddDataHandler(mux, baseurl, uiPath, datadir)
	restRunPath := uiPath + RestRunURI
	mux.HandleFunc(restRunPath, RESTRunHandler)
	restStatusPath := uiPath + RestStatusURI
	mux.HandleFunc(restStatusPath, RESTStatusHandler)
	restStopPath := uiPath + RestStopURI
	mux.HandleFunc(restStopPath, RESTStopHandler)
	dnsPath := uiPath + RestDNS
	mux.HandleFunc(dnsPath, RESTDNSHandler)
	log.Printf("REST API on %s, %s, %s, %s", restRunPath, restStatusPath, restStopPath, dnsPath)
}

// SaveJSON save Json bytes to give file name (.json) in data-path dir.
func SaveJSON(name string, json []byte) string {
	if dataDir == "" {
		log.Infof("Not saving because data-path is unset")
		return ""
	}
	name += ".json"
	log.Infof("Saving %s in %s", name, dataDir)
	err := os.WriteFile(path.Join(dataDir, name), json, 0o644) //nolint:gosec // we do want 644
	if err != nil {
		log.Errf("Unable to save %s in %s: %v", name, dataDir, err)
		return ""
	}
	// Return the relative path from the /fortio/ UI
	return "data/" + name
}

func NextRunID() int64 {
	uiRunMapMutex.Lock()
	id++ // start at 1 as 0 means interrupt all
	runid := id
	runs[runid] = &Status{State: StatePending, RunID: runid}
	uiRunMapMutex.Unlock()
	return runid
}

// Must be called exactly once for each runner. Responsible for normalization (abort channel setup)
// and making sure the options object returned in status is same as the actual one.
// Note that the Aborter/Stop field is being "moved" into the runner when making the concrete runner
// and cleared from the original options object so we need to keep our own copy of the aborter pointer.
// See newPeriodicRunner. Note this is arguably not the best behavior design/could be changed.
func UpdateRun(ro *periodic.RunnerOptions) *periodic.Aborter {
	uiRunMapMutex.Lock()
	status, found := runs[ro.RunID]
	if !found || status.State != StatePending || status.RunnerOptions != nil || status.RunID != ro.RunID {
		uiRunMapMutex.Unlock()
		// This would be a bug so we crash:
		log.Fatalf("Logic bug: updating unexpected state for rid %d: %v", ro.RunID, status)
	}
	status.State = StateRunning
	status.RunnerOptions = ro
	status.RunnerOptions.Normalize()
	status.aborter = status.RunnerOptions.Stop // save the aborter before it gets cleared in newPeriodicRunner.
	uiRunMapMutex.Unlock()
	return status.aborter
}

func GetRun(id int64) *Status {
	uiRunMapMutex.Lock()
	res := runs[id] // TODO: race: need to copy because the pointers to options can be changed by the runners
	uiRunMapMutex.Unlock()
	return res
}

// GetAllRuns returns a copy of the status map
// (note maps are always reference types so no copy is done when returning the map value).
func GetAllRuns() StatusMap {
	// make a copy - we could use the hint of the size but that would require locking
	res := make(StatusMap)
	uiRunMapMutex.Lock()
	for k, v := range runs {
		res[k] = v
	}
	uiRunMapMutex.Unlock()
	return res
}

// RunMetrics returns the number of currently running runs as well as the total runs so far.
// acquire the lock once (per scrape) for all stats.
func RunMetrics() (int, int64) {
	uiRunMapMutex.Lock()
	runid := id
	n := len(runs)
	uiRunMapMutex.Unlock()
	return n, runid
}

func SetDataDir(datadir string) {
	dataDir = datadir
}

func GetDataDir() string {
	return dataDir
}
