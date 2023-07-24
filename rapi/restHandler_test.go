// Copyright 2022 Fortio Authors.
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

package rapi

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"fortio.org/fortio/fgrpc"
	"fortio.org/fortio/fhttp"
	"fortio.org/fortio/fnet"
	"fortio.org/fortio/jrpc"
	"fortio.org/fortio/periodic"
	"fortio.org/fortio/tcprunner"
	"fortio.org/fortio/udprunner"
	"fortio.org/log"
)

// Generics ftw.
func FetchResult[T any](t *testing.T, url string, jsonPayload string) *T {
	r, err := jrpc.Fetch[T](jrpc.NewDestination(url), []byte(jsonPayload))
	if err != nil {
		t.Errorf("Got unexpected error for URL %s: %v - %v", url, err, r)
	}
	return r
}

func GetResult(t *testing.T, url string, jsonPayload string) *fhttp.HTTPRunnerResults {
	return FetchResult[fhttp.HTTPRunnerResults](t, url, jsonPayload)
}

// Same as above but when expecting to get an Async reply.
func GetAsyncResult(t *testing.T, url string, jsonPayload string) *AsyncReply {
	r := FetchResult[AsyncReply](t, url, jsonPayload)
	if r == nil {
		t.Fatalf("Unexpected nil reply")
		return r
	}
	if r.Error {
		t.Errorf("Unexpected false success field: +%v", r)
	}
	return r
}

// Same as above but when expecting to get an error reply.
func GetErrorResult(t *testing.T, url string, jsonPayload string) *jrpc.ServerReply {
	r, err := jrpc.Fetch[jrpc.ServerReply](jrpc.NewDestination(url), []byte(jsonPayload))
	if err == nil {
		t.Errorf("Got unexpected no error for URL %s: %v", url, r)
	}
	if r == nil {
		t.Fatalf("Unexpected nil error reply")
	}
	if !r.Error {
		t.Error("Success field should be false for errors")
	}
	var fe *jrpc.FetchError
	if !errors.As(err, &fe) {
		t.Errorf("Error isn't a FetchError for URL %s: %v", url, err)
	}
	// including -1 which would be a low level error, we expect an actual 4xx/5xx
	if fe != nil && fe.Code < 300 {
		t.Errorf("Got unexpected http code %d: %v", fe.Code, fe)
	}
	return r
}

func hookTest(_ *fhttp.HTTPOptions, _ *periodic.RunnerOptions) {
	// TODO: find something to mutate/test
}

//nolint:funlen,gocognit,maintidx // it's a test of a lot of things in sequence/context
func TestHTTPRunnerRESTApi(t *testing.T) {
	// log.SetLogLevel(log.Verbose)
	mux, addr := fhttp.DynamicHTTPServer(false)
	mux.HandleFunc("/foo/", fhttp.EchoHandler)
	baseURL := fmt.Sprintf("http://localhost:%d/", addr.Port)
	uiPath := "/fortioCustom/"
	tmpDir := t.TempDir()
	os.Create(path.Join(tmpDir, "foo.txt")) // not a json, will be skipped over
	badJSON := path.Join(tmpDir, "bad.json")
	os.Create(badJSON)
	err := os.Chmod(badJSON, 0) // make the file un readable so it should also be skipped (doesn't work on ci(!))
	if err != nil {
		t.Errorf("Unable to make file unreadable, will make test about bad.json fail later: %v", err)
	}
	AddHandlers(hookTest, mux, "", uiPath, tmpDir)

	dnsURL := fmt.Sprintf("http://localhost:%d%s%s", addr.Port, uiPath, RestDNS)
	// Error case (no/empty name)
	reply, err := jrpc.Get[DNSReply](jrpc.NewDestination(dnsURL))
	if err == nil {
		t.Errorf("Got unexpected no error for URL %s: %+v", dnsURL, reply)
	}
	if !reply.Error {
		t.Errorf("Unexpected no error field: %+v", reply)
	}
	// Ok case
	dnsOkURL := dnsURL + "?name=debug.fortio.org"
	reply, err = jrpc.Get[DNSReply](jrpc.NewDestination(dnsOkURL))
	if err != nil {
		t.Errorf("Got unexpected error for URL %s: %v - %+v", dnsOkURL, err, reply)
	}
	if reply.Error {
		t.Errorf("Unexpected error field: %+v", reply)
	}
	if reply.Name != "debug.fortio.org" {
		t.Errorf("Unexpected name no echoed back: %+v", reply)
	}
	// test need change when we change debug.fortio.org hosting (currently 3 hosts with both ipv4 and ipv6)
	if len(reply.IPv4) != len(reply.IPv6) || len(reply.IPv4) != 3 {
		t.Errorf("Unexpected number of IPs (3 currently for debug.fortio.org): %+v", reply)
	}

	restURL := fmt.Sprintf("http://localhost:%d%s%s", addr.Port, uiPath, RestRunURI)

	runURL := fmt.Sprintf("%s?qps=%d&url=%s&t=2s", restURL, 100, baseURL)

	res := GetResult(t, runURL, "")
	if res.RetCodes[200] != 0 {
		t.Errorf("Got unexpected 200s %d on base: %+v", res.RetCodes[200], res)
	}
	if res.RetCodes[404] != 2*100 { // 2s at 100qps == 200
		t.Errorf("Got unexpected 404s count %d on base: %+v", res.RetCodes[404], res)
	}
	echoURL := baseURL + "foo/bar?delay=20ms&status=200:100"
	runURL = fmt.Sprintf("%s?qps=%d&url=%s&n=200", restURL, 100, echoURL)
	res = GetResult(t, runURL, "")
	totalReq := res.DurationHistogram.Count
	httpOk := res.RetCodes[http.StatusOK]
	if totalReq != httpOk {
		t.Errorf("Mismatch between requests %d and ok %v (%+v)", totalReq, res.RetCodes, res)
	}
	if res.SocketCount != int64(res.RunnerResults.NumThreads) {
		t.Errorf("%d socket used, expected same as thread# %d", res.SocketCount, res.RunnerResults.NumThreads)
	}

	// Check payload is used and that query arg overrides payload
	jsonData := fmt.Sprintf("{\"metadata\": {\"url\":%q, \"save\":\"on\", \"n\":\"200\", \"payload\": \"test payload\"}}", echoURL)
	runURL = fmt.Sprintf("%s?jsonPath=.metadata&qps=100&n=100", restURL)
	res = GetResult(t, runURL, jsonData)
	totalReq = res.DurationHistogram.Count
	httpOk = res.RetCodes[http.StatusOK]
	if totalReq != httpOk {
		t.Errorf("Mismatch between requests %d and ok %v (%+v)", totalReq, res.RetCodes, res)
	}
	if totalReq != 100 {
		t.Errorf("Precedence error, value in url query arg (n=100) should be used, we got %d", totalReq)
	}
	savedID := res.RunID
	if savedID <= 0 {
		t.Errorf("Saved id should be >=1: %d", savedID)
	}

	// Send a bad (missing unit) duration (test error return)
	runURL = fmt.Sprintf("%s?jsonPath=.metadata&qps=100&n=10&t=42", restURL)
	errObj := GetErrorResult(t, runURL, jsonData)
	if errObj.Message != "parsing duration" || errObj.Exception != "time: missing unit in duration \"42\"" {
		t.Errorf("Didn't get the expected duration parsing error, got %+v", errObj)
	}
	// bad json path: doesn't exist
	runURL = fmt.Sprintf("%s?jsonPath=.foo", restURL)
	errObj = GetErrorResult(t, runURL, jsonData)
	if errObj.Exception != "\"foo\" not found in json" {
		t.Errorf("Didn't get the expected json body access error, got %+v", errObj)
	}
	// bad json path: wrong type
	runURL = fmt.Sprintf("%s?jsonPath=.metadata.url", restURL)
	errObj = GetErrorResult(t, runURL, jsonData)
	if errObj.Exception != "\"url\" path is not a map" {
		t.Errorf("Didn't get the expected json type mismatch error, got %+v", errObj)
	}
	// missing url and a few other cases
	jsonData = `{"metadata": {"n": 200}}`
	runURL = fmt.Sprintf("%s?jsonPath=.metadata", restURL)
	errObj = GetErrorResult(t, runURL, jsonData)
	if errObj.Message != "URL is required" {
		t.Errorf("Didn't get the expected url missing error, got %+v", errObj)
	}
	// not well formed json
	jsonData = `{"metadata": {"n":`
	runURL = fmt.Sprintf("%s?jsonPath=.metadata", restURL)
	errObj = GetErrorResult(t, runURL, jsonData)
	if errObj.Exception != "unexpected end of JSON input" {
		t.Errorf("Didn't get the expected error for truncated/invalid json, got %+v", errObj)
	}
	// Exercise Hearders code (but hard to test the effect,
	// would need to make a single echo query instead of a run... which the API doesn't do)
	jsonData = `{"metadata": {"headers": ["Foo: Bar", "Blah: BlahV"]}}`
	runURL = fmt.Sprintf("%s?jsonPath=.metadata&qps=90&n=23&url=%s&H=Third:HeaderV", restURL, echoURL)
	res = GetResult(t, runURL, jsonData)
	if res.RetCodes[http.StatusOK] != 23 {
		t.Errorf("Should have done expected 23 requests, got %+v", res.RetCodes)
	}
	// Start infinite running run
	runURL = fmt.Sprintf("%s?jsonPath=.metadata&qps=4.20&t=on&url=%s&async=on&save=on", restURL, echoURL)
	asyncObj := GetAsyncResult(t, runURL, jsonData)
	runID := asyncObj.RunID
	if asyncObj.Message != "started" || runID <= savedID {
		t.Errorf("Should started async job got %+v", asyncObj)
	}
	fileID := asyncObj.ResultID
	if fileID == "" {
		t.Errorf("unexpected empty resultid")
	}
	fileURL := asyncObj.ResultURL
	if fileURL == "" {
		t.Errorf("unexpected empty file URL")
	}
	// Get status
	statusURL := fmt.Sprintf("http://localhost:%d%s%s?runid=%d", addr.Port, uiPath, RestStatusURI, runID)
	statusDest := &jrpc.Destination{
		URL:     statusURL,
		Timeout: 3 * time.Second,
	}
	statuses, err := jrpc.Get[StatusReply](statusDest)
	if err != nil {
		t.Errorf("Error getting status %q: %v", statusURL, err)
	}
	if len(statuses.Statuses) != 1 {
		t.Errorf("Status count %d != expected 1", len(statuses.Statuses))
	}
	if c, _ := RunMetrics(); c != 1 {
		t.Errorf("NumRuns() %d != expected 1", c)
	}
	status, found := statuses.Statuses[runID]
	if !found {
		t.Errorf("Status not found in reply, for runid %d: %+v", runID, statuses)
	}
	// Could in theory be pending if really fast
	if status.State != StateRunning {
		t.Errorf("Expected status to be running, got %q", status.State.String())
	}
	if status.RunID != runID {
		t.Errorf("Status runid %d != expected %d", status.RunID, runID)
	}
	if status.RunnerOptions.QPS != 4.20 {
		t.Errorf("Expected to see request as sent (4.2), got: %+v", status)
	}
	if status.RunnerOptions.RunType != "HTTP" {
		t.Errorf("RunType mismatch, got: %+v", status.RunnerOptions)
	}
	if status.RunnerOptions.ID != fileID {
		t.Errorf("Mismatch between ids from start %q vs status %q", fileID, status.RunnerOptions.ID)
	}
	// And stop it (with wait so data is there when it returns):
	stopURL := fmt.Sprintf("http://localhost:%d%s%s?runid=%d&wait=true", addr.Port, uiPath, RestStopURI, runID)
	asyncObj = GetAsyncResult(t, stopURL, "")
	if asyncObj.Message != StateStopped.String() ||
		asyncObj.RunID != runID || asyncObj.Count != 1 || asyncObj.ResultID != fileID {
		t.Errorf("Should have stopped matching async job got %+v", asyncObj)
	}
	// Fetch the result: (right away, racing with stop above which thus must be synchronous)
	fetchURL := fmt.Sprintf("http://localhost:%d%sdata/%s.json", addr.Port, uiPath, fileID)
	if fetchURL != fileURL {
		t.Errorf("Unexpected mismatch between calculated fetchURL %q and returned one %q", fetchURL, fileURL)
	}
	res = GetResult(t, fetchURL, "")
	if res.RequestedQPS != "4.2" {
		t.Errorf("Not the expected requested qps %q", res.RequestedQPS)
	}
	if res.RequestedDuration != "until stop" {
		t.Errorf("Not the expected requested duration %q", res.RequestedDuration)
	}
	totalReq = res.DurationHistogram.Count
	httpOk = res.RetCodes[http.StatusOK]
	if totalReq != httpOk {
		t.Errorf("Mismatch between requests %d and ok %v (%+v)", totalReq, res.RetCodes, res)
	}
	if res.Result().ID != fileID {
		t.Errorf("Mismatch between ids %q vs result %q", fileID, res.Result().ID)
	}
	// (Also for that run:) Stop it again, should be 0 count
	asyncObj = GetAsyncResult(t, stopURL, "")
	if asyncObj.Message != StateStopping.String() || asyncObj.RunID != runID || asyncObj.Count != 0 {
		t.Errorf("2nd stop should be noop, got %+v", asyncObj)
	}
	// Status should be empty (nothing running)
	statuses, err = jrpc.Get[StatusReply](statusDest)
	if err != nil {
		t.Errorf("Error getting status %q: %v", statusURL, err)
	}
	if len(statuses.Statuses) != 0 {
		t.Errorf("Status count %d != expected 0 - %v", len(statuses.Statuses), statuses)
	}
	if c, n := RunMetrics(); c != 0 || n < 1 {
		t.Errorf("NumRuns() %d,%d != expected 0,>=1", c, n)
	}
	// Start 3 async test
	runURL = fmt.Sprintf("%s?jsonPath=.metadata&qps=1&t=on&url=%s&async=on", restURL, echoURL)
	_ = GetAsyncResult(t, runURL, jsonData)
	_ = GetAsyncResult(t, runURL, jsonData)
	_ = GetAsyncResult(t, runURL, jsonData)
	// Get all statuses
	statusURL = fmt.Sprintf("http://localhost:%d%s%s", addr.Port, uiPath, RestStatusURI)
	statusDest.URL = statusURL
	statuses, err = jrpc.Get[StatusReply](statusDest)
	if err != nil {
		t.Errorf("Error getting status %q: %v", statusURL, err)
	}
	if len(statuses.Statuses) != 3 {
		t.Errorf("Status count not the expected 3: %+v", statuses)
	}
	if c, n := RunMetrics(); c != 3 || n < 4 {
		t.Errorf("NumRuns() %d,%d != expected 3,>=4", c, n)
	}
	// stop all
	stopURL = fmt.Sprintf("http://localhost:%d%s%s", addr.Port, uiPath, RestStopURI)
	asyncObj = GetAsyncResult(t, stopURL, "")
	if asyncObj.Message != StateStopping.String() || asyncObj.RunID != 0 || asyncObj.Count != 3 {
		t.Errorf("Should have stopped 3 async job got %+v", asyncObj)
	}

	// add one more with bad url
	badURL := fmt.Sprintf("%s?jsonPath=.metadata&qps=1&t=on&url=%s&async=on", restURL, "http://doesnotexist.fortio.org/TestHTTPRunnerRESTApi")
	asyncObj = GetAsyncResult(t, badURL, jsonData)
	runID = asyncObj.RunID
	if asyncObj.Message != "started" || runID <= savedID+5 { // 1+1+3 jobs before this one
		t.Errorf("Should started async job got %+v", asyncObj)
	}

	tsvURL := fmt.Sprintf("http://localhost:%d%s%s", addr.Port, uiPath, "data/index.tsv")
	code, bytes, err := jrpc.FetchURL(tsvURL)
	if err != nil {
		t.Errorf("Unexpected error for %s: %v", tsvURL, err)
	}
	if code != http.StatusOK {
		t.Errorf("Error getting tsv index: %d", code)
	}
	str := string(bytes)
	// Check that the runid from above made it to the list
	runStr := fmt.Sprintf("http://localhost:%d%sdata/%s.json\t", addr.Port, uiPath, fileID)
	if !strings.Contains(str, runStr) {
		t.Errorf("Expected to find %q in %q", runStr, str)
	}
	if strings.Contains(str, "foo.txt") {
		t.Errorf("Result of index.tsv should not include non .json files: %s", str)
	}
	// Note this test fails if running as root.
	if strings.Contains(str, "bad.json") {
		t.Errorf("Result of index.tsv should not include unreadble .json files (%q): %s", badJSON, str)
	}
	files := DataList()
	if len(files) < 1 {
		t.Error("DataList() should also return files when dir is correct")
	}
	// Check we can't fetch the foo.txt
	fetchTxt := fmt.Sprintf("http://localhost:%d%sdata/foo.txt", addr.Port, uiPath)
	code, bytes, err = jrpc.FetchURL(fetchTxt)
	if err != nil {
		t.Errorf("Unexpected error in fetch %q: %v", fetchTxt, err)
	}
	if code != http.StatusNotFound {
		t.Errorf("foo.txt should have been not found, got %d %s", code, fnet.DebugSummary(bytes, 256))
	}
	SetDataDir("/does/not/exist")
	code, bytes, err = jrpc.FetchURL(tsvURL)
	if err != nil {
		t.Errorf("Unexpected low level error for %s: %v", tsvURL, err)
	}
	if code != http.StatusServiceUnavailable {
		t.Errorf("Setting bad directory should error out, it didn't - got %s", jrpc.DebugSummary(bytes, 256))
	}
	none := DataList()
	if len(none) > 0 {
		t.Errorf("Setting bad directory should not get any files got %v", none)
	}
}

//nolint:funlen
func TestRESTStopTimeBased(t *testing.T) {
	log.SetLogLevel(log.Verbose)
	mux, addr := fhttp.DynamicHTTPServer(false)
	mux.HandleFunc("/foo/", fhttp.EchoHandler)
	baseURL := fmt.Sprintf("http://localhost:%d/", addr.Port)
	uiPath := "/fortio3/"
	tmpDir := t.TempDir()
	AddHandlers(nil, mux, "https://foo.fortio.org", uiPath, tmpDir)
	restURL := fmt.Sprintf("http://localhost:%d%s%s", addr.Port, uiPath, RestRunURI)
	echoURL := baseURL + "foo/bar?delay=20ms&status=200:100"
	// Start infinite running run
	runURL := fmt.Sprintf("%s?jsonPath=.metadata&qps=4.20&t=on&url=%s&async=on&save=on", restURL, echoURL)
	asyncObj := GetAsyncResult(t, runURL, "")
	runID := asyncObj.RunID
	if asyncObj.Message != "started" || runID <= 0 {
		t.Errorf("Should started async job got %+v", asyncObj)
	}
	fileID := asyncObj.ResultID
	if fileID == "" {
		t.Errorf("unexpected empty resultid")
	}
	fileURL := asyncObj.ResultURL
	if fileURL == "" {
		t.Errorf("unexpected empty file URL")
	}
	if !strings.HasPrefix(fileURL, "https://foo.fortio.org/fortio3/data/") {
		t.Errorf("Fetch URL when there is a baseURL should start with it, got %q", fileURL)
	}
	// Get status
	statusURL := fmt.Sprintf("http://localhost:%d%s%s?runid=%d", addr.Port, uiPath, RestStatusURI, runID)
	statusDest := jrpc.NewDestination(statusURL)
	statuses, err := jrpc.Get[StatusReply](statusDest)
	if err != nil {
		t.Errorf("Error getting status %q: %v", statusURL, err)
	}
	if len(statuses.Statuses) != 1 {
		t.Errorf("Status count %d != expected 1", len(statuses.Statuses))
	}
	status, found := statuses.Statuses[runID]
	if !found {
		t.Errorf("Status not found in reply, for runid %d: %+v", runID, statuses)
	}
	// Could in theory be pending if really fast
	if status.State != StateRunning {
		t.Errorf("Expected status to be running, got %q", status.State.String())
	}
	if status.RunID != runID {
		t.Errorf("Status runid %d != expected %d", status.RunID, runID)
	}
	if status.RunnerOptions.QPS != 4.20 {
		t.Errorf("Expected to see request as sent (4.2), got: %+v", status)
	}
	if status.RunnerOptions.RunType != "HTTP" {
		t.Errorf("RunType mismatch, got: %+v", status.RunnerOptions)
	}
	if status.RunnerOptions.ID != fileID {
		t.Errorf("Mismatch between ids from start %q vs status %q", fileID, status.RunnerOptions.ID)
	}
	// And stop it (with wait so data is there when it returns):
	stopURL := fmt.Sprintf("http://localhost:%d%s%s?runid=%d&wait=false", addr.Port, uiPath, RestStopURI, runID)
	stopping := StateStopping.String()
	asyncObj = GetAsyncResult(t, stopURL, "")
	if asyncObj.Message != stopping || asyncObj.RunID != runID || asyncObj.Count != 1 || asyncObj.ResultID != fileID {
		t.Errorf("Should have stopped matching async job got %+v", asyncObj)
	}
	// Wait/give it time to really stop
	time.Sleep(3 * time.Second)
	// Stop it again, should be 0 count
	asyncObj = GetAsyncResult(t, stopURL, "")
	if asyncObj.Message != stopping || asyncObj.RunID != runID || asyncObj.Count != 0 {
		t.Errorf("2nd stop should be noop, got %+v", asyncObj)
	}
	// Status should be empty (nothing running)
	statuses, err = jrpc.Get[StatusReply](statusDest)
	if err != nil {
		t.Errorf("Error getting status %q: %v", statusURL, err)
	}
	if len(statuses.Statuses) != 0 {
		bytes, _ := jrpc.Serialize(statuses.Statuses)
		t.Errorf("Status count %d != expected 0 - %s", len(statuses.Statuses), string(bytes))
	}
	// Fetch the result:
	fetchURL := fmt.Sprintf("http://localhost:%d%sdata/%s.json", addr.Port, uiPath, fileID)
	res := GetResult(t, fetchURL, "")
	if res.RequestedQPS != "4.2" {
		t.Errorf("Not the expected requested qps %q", res.RequestedQPS)
	}
	if res.RequestedDuration != "until stop" {
		t.Errorf("Not the expected requested duration %q", res.RequestedDuration)
	}
	totalReq := res.DurationHistogram.Count
	httpOk := res.RetCodes[http.StatusOK]
	if totalReq != httpOk {
		t.Errorf("Mismatch between requests %d and ok %v (%+v)", totalReq, res.RetCodes, res)
	}
	if res.Result().ID != fileID {
		t.Errorf("Mismatch between ids %q vs result %q", fileID, res.Result().ID)
	}
	tsvURL := fmt.Sprintf("http://localhost:%d%s%s", addr.Port, uiPath, "data/index.tsv")
	code, bytes, err := jrpc.FetchURL(tsvURL)
	if err != nil {
		t.Errorf("Unexpected error for %s: %v", tsvURL, err)
	}
	if code != http.StatusOK {
		t.Errorf("Error getting tsv index: %d", code)
	}
	dataStr := string(bytes)
	if !strings.Contains(dataStr, "https://foo.fortio.org/fortio3/data/") {
		t.Errorf("Base url not found in result %s", dataStr)
	}
	indexURL := fmt.Sprintf("http://localhost:%d%s%s", addr.Port, uiPath, "data/index.html")
	code, bytes, err = jrpc.FetchURL(indexURL)
	if err != nil {
		t.Errorf("Unexpected error for %s: %v", indexURL, err)
	}
	if code != http.StatusOK {
		t.Errorf("Error getting html index: %d", code)
	}
	dataStr = string(bytes)
	if strings.Contains(dataStr, "https://foo.fortio.org/fortio/data/") {
		t.Errorf("Base url should not be found in html result %s", dataStr)
	}
	expected := fmt.Sprintf("<li><a href=\"%s.json\">%s</a>", fileID, fileID)
	if !strings.Contains(dataStr, expected) {
		t.Errorf("Can't find expected html %s in %s", expected, dataStr)
	}
}

// Test the bad host case #796.
func TestHTTPRunnerRESTApiBadHost(t *testing.T) {
	log.SetLogLevel(log.Debug) // needed to debug if this test starts failing
	// otherwise log.SetLogLevel(log.Info)
	mux, addr := fhttp.DynamicHTTPServer(false)
	uiPath := "/f/"
	AddHandlers(nil, mux, "", uiPath, ".")
	// Error with bad host
	restURL := fmt.Sprintf("http://localhost:%d%s%s", addr.Port, uiPath, RestRunURI)
	// sync first:
	runURL := fmt.Sprintf("%s?qps=%d&url=%s&t=2s", restURL, 100, "http://doesnotexist.fortio.org/TestHTTPRunnerRESTApiBadHost/foo/bar")
	errObj := GetErrorResult(t, runURL, "")
	// we get either `lookup doesnotexist.fortio.org: no such host` or `lookup doesnotexist.fortio.org on 127.0.0.11:53: no such host`
	// so check just for prefix
	if !strings.HasPrefix(errObj.Exception, "lookup doesnotexist.fortio.org") {
		t.Errorf("Didn't get the expected dns error, got %+v", errObj)
	}
	// Same with async:
	runURL += "&async=on&save=on"
	asyncRes := GetAsyncResult(t, runURL, "")
	dataURL := asyncRes.ResultURL
	if dataURL == "" {
		t.Errorf("Expected a result URL, got %+v", asyncRes)
	}
	runID := asyncRes.RunID
	if runID < 1 {
		t.Errorf("Expected a run id, got %+v", asyncRes)
	}
	// And stop it (with wait to avoid race condition/so data is here when this returns and avoid a sleep)
	stopURL := fmt.Sprintf("http://localhost:%d%s%s?runid=%d&wait=true", addr.Port, uiPath, RestStopURI, runID)
	prevTimeout := jrpc.SetCallTimeout(1 * time.Second) // Stopping a failed to start run should be almost instant
	asyncRes = GetAsyncResult(t, stopURL, "")
	// Restore previous one (60s). Note we could also change GetAsyncResult to take a jrpc.Destination with timeout but that's more change for just a test.
	jrpc.SetCallTimeout(prevTimeout)
	if asyncRes.ResultURL != dataURL {
		t.Errorf("Expected same result URL, got %+v", asyncRes)
	}
	// Fetch the json result:
	res := GetResult(t, dataURL, "")
	if !strings.HasPrefix(res.Exception, "lookup doesnotexist.fortio.org") {
		t.Errorf("Didn't get the expected dns error in result file url %s, got %+v", asyncRes.ResultURL, res)
	}
}

// If jsonPayload isn't empty we POST otherwise get the url.
func GetGRPCResult(t *testing.T, url string, jsonPayload string) *fgrpc.GRPCRunnerResults {
	r, err := jrpc.Fetch[fgrpc.GRPCRunnerResults](jrpc.NewDestination(url), []byte(jsonPayload))
	if err != nil {
		t.Errorf("Got unexpected err for URL %s: %v", url, err)
	}
	return r
}

func TestOtherRunnersRESTApi(t *testing.T) {
	iPort := fgrpc.PingServerTCP("0", "bar", 0, &fhttp.TLSOptions{})
	iDest := fmt.Sprintf("localhost:%d", iPort)

	mux, addr := fhttp.DynamicHTTPServer(false)
	AddHandlers(nil, mux, "", "/fortio/", ".")
	restURL := fmt.Sprintf("http://localhost:%d/fortio/rest/run", addr.Port)

	runURL := fmt.Sprintf("%s?qps=%d&url=%s&t=2s&runner=grpc", restURL, 10, iDest)

	res := FetchResult[fgrpc.GRPCRunnerResults](t, runURL, "")
	totalReq := res.DurationHistogram.Count
	httpOk := res.RetCodes["SERVING"]
	if totalReq != httpOk {
		t.Errorf("Mismatch between grpc requests %d and ok %v (%+v)",
			totalReq, res.RetCodes, res)
	}

	tAddr := fnet.TCPEchoServer("test-echo-runner-tcp", ":0")
	tDest := fmt.Sprintf("tcp://localhost:%d/", tAddr.(*net.TCPAddr).Port)
	runURL = fmt.Sprintf("%s?qps=%d&url=%s&t=2s&c=2", restURL, 10, tDest)

	tRes := FetchResult[tcprunner.RunnerResults](t, runURL, "")
	if tRes.ActualQPS < 8 || tRes.ActualQPS > 10.1 {
		t.Errorf("Unexpected tcp qps %f", tRes.ActualQPS)
	}

	uAddr := fnet.UDPEchoServer("test-echo-runner-udp", ":0", false)
	uDest := fmt.Sprintf("udp://localhost:%d/", uAddr.(*net.UDPAddr).Port)
	runURL = fmt.Sprintf("%s?qps=%d&url=%s&t=2s&c=1", restURL, 5, uDest)

	uRes := FetchResult[udprunner.RunnerResults](t, runURL, "")
	if uRes.ActualQPS < 4 || uRes.ActualQPS > 5.1 {
		t.Errorf("Unexpected udp qps %f", tRes.ActualQPS)
	}
}

func TestNextGet(t *testing.T) {
	id := NextRunID()
	ro := GetRun(id)
	bytes, err := jrpc.Serialize(ro)
	if err != nil {
		t.Errorf("Unexpected error serializing %+v: %v", ro, err)
	}
	str := string(bytes)
	expected := fmt.Sprintf("{\"RunID\":%d,\"State\":%d,\"RunnerOptions\":null}", id, StatePending)
	if str != expected {
		t.Errorf("Expected json %s got %s", expected, str)
	}
	list := GetAllRuns()
	t.Logf("expecting only %d", id)
	for _, r := range list {
		t.Logf("run %d: %+v", r.RunID, r)
	}
	if len(list) != 1 {
		t.Errorf("Expected 1 run got %d", len(list))
	}
}

func TestDataDir(t *testing.T) {
	oldDir := GetDataDir()
	SetDataDir("")
	fname := SaveJSON("foo.json", []byte{})
	if fname != "" {
		t.Errorf("expected error on empty/unset dir, got %q", fname)
	}
	SetDataDir("/does/not/exist")
	fname = SaveJSON("bar.json", []byte{})
	if fname != "" {
		t.Errorf("expected error on invalid dir, got %q", fname)
	}
	SetDataDir(oldDir)
}
