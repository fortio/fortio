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

	"fortio.org/fortio/fgrpc"
	"fortio.org/fortio/fhttp"
	"fortio.org/fortio/fnet"
	"fortio.org/fortio/jrpc"
	"fortio.org/fortio/tcprunner"
	"fortio.org/fortio/udprunner"
)

// Generics ftw.
func FetchResult[T any](t *testing.T, url string, jsonPayload string) *T {
	r, err := jrpc.CallWithPayload[T](url, []byte(jsonPayload))
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
	if r.Failed {
		t.Errorf("Unexpected false success field: +%v", r)
	}
	return r
}

// Same as above but when expecting to get an error reply.
func GetErrorResult(t *testing.T, url string, jsonPayload string) *jrpc.ErrorReply {
	r, err := jrpc.CallWithPayload[jrpc.ErrorReply](url, []byte(jsonPayload))
	if err == nil {
		t.Errorf("Got unexpected no error for URL %s: %v", url, r)
	}
	if !r.Failed {
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

// nolint: funlen,gocognit,maintidx // it's a test of a lot of things in sequence/context
func TestHTTPRunnerRESTApi(t *testing.T) {
	mux, addr := fhttp.DynamicHTTPServer(false)
	mux.HandleFunc("/foo/", fhttp.EchoHandler)
	baseURL := fmt.Sprintf("http://localhost:%d/", addr.Port)
	uiPath := "/fortio/"
	tmpDir := t.TempDir()
	os.Create(path.Join(tmpDir, "foo.txt")) // not a json, will be skipped over
	badJSON := path.Join(tmpDir, "bad.json")
	os.Create(badJSON)
	err := os.Chmod(badJSON, 0) // make the file un readable so it should also be skipped (doesn't work on ci(!))
	if err != nil {
		t.Errorf("Unable to make file unreadable, will make test about bad.json fail later: %v", err)
	}
	AddHandlers(mux, uiPath, tmpDir)
	mux.HandleFunc("/data/index.tsv", func(w http.ResponseWriter, r *http.Request) { SendTSVDataIndex("/data/", w) })

	restURL := fmt.Sprintf("http://localhost:%d%s%s", addr.Port, uiPath, restRunURI)

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
	if res.SocketCount != res.RunnerResults.NumThreads {
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
	runURL = fmt.Sprintf("%s?jsonPath=.metadata&qps=10&t=on&url=%s&async=on", restURL, echoURL)
	asyncObj := GetAsyncResult(t, runURL, jsonData)
	runID := asyncObj.RunID
	if asyncObj.Message != "started" || runID <= savedID {
		t.Errorf("Should started async job got %+v", asyncObj)
	}
	// And stop it:
	stopURL := fmt.Sprintf("http://localhost:%d%s%s?runid=%d", addr.Port, uiPath, restStopURI, runID)
	asyncObj = GetAsyncResult(t, stopURL, "")
	stoppedMsg := "stopped"
	if asyncObj.Message != stoppedMsg || asyncObj.RunID != runID || asyncObj.Count != 1 {
		t.Errorf("Should have stopped async job got %+v", asyncObj)
	}
	// Stop it again, should be 0 count
	asyncObj = GetAsyncResult(t, stopURL, "")
	if asyncObj.Message != stoppedMsg || asyncObj.RunID != runID || asyncObj.Count != 0 {
		t.Errorf("2nd stop should be noop, got %+v", asyncObj)
	}
	// Start 3 async test and stop all
	runURL = fmt.Sprintf("%s?jsonPath=.metadata&qps=1&t=on&url=%s&async=on", restURL, echoURL)
	_ = GetAsyncResult(t, runURL, jsonData)
	_ = GetAsyncResult(t, runURL, jsonData)
	_ = GetAsyncResult(t, runURL, jsonData)
	stopURL = fmt.Sprintf("http://localhost:%d%s%s", addr.Port, uiPath, restStopURI)
	asyncObj = GetAsyncResult(t, stopURL, "")
	if asyncObj.Message != stoppedMsg || asyncObj.RunID != 0 || asyncObj.Count != 3 {
		t.Errorf("Should have stopped 3 async job got %+v", asyncObj)
	}

	// add one more with bad url
	badURL := fmt.Sprintf("%s?jsonPath=.metadata&qps=1&t=on&url=%s&async=on", restURL, "http://doesnotexist.fortio.org/")
	asyncObj = GetAsyncResult(t, badURL, jsonData)
	runID = asyncObj.RunID
	if asyncObj.Message != "started" || runID <= savedID+5 { // 1+1+3 jobs before this one
		t.Errorf("Should started async job got %+v", asyncObj)
	}

	tsvURL := fmt.Sprintf("http://localhost:%d%s", addr.Port, "/data/index.tsv")
	code, bytes, err := jrpc.Fetch(tsvURL)
	if err != nil {
		t.Errorf("Unexpected error for %s: %v", tsvURL, err)
	}
	if code != http.StatusOK {
		t.Errorf("Error getting tsv index: %d", code)
	}
	str := string(bytes)
	// Check that the runid from above made it to the list
	runStr := fmt.Sprintf("_%d.json\t", savedID)
	if !strings.Contains(str, runStr) {
		t.Errorf("Expected to find %q in %q", runStr, str)
	}
	if strings.Contains(str, "foo.txt") {
		t.Errorf("Result of index.tsv should not include non .json files: %s", str)
	}
	if os.Getenv("CIRCLECI") == "" {
		// Somehow this test fails on Circle CI (file is readable despite chmod...)
		if strings.Contains(str, "bad.json") {
			t.Errorf("Result of index.tsv should not include unreadble .json files (%q): %s", badJSON, str)
		}
	}
	files := DataList()
	if len(files) < 1 {
		t.Error("DataList() should also return files when dir is correct")
	}
	SetDataDir("/does/not/exist")
	code, bytes, err = jrpc.Fetch(tsvURL)
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

// If jsonPayload isn't empty we POST otherwise get the url.
func GetGRPCResult(t *testing.T, url string, jsonPayload string) *fgrpc.GRPCRunnerResults {
	r, err := jrpc.CallWithPayload[fgrpc.GRPCRunnerResults](url, []byte(jsonPayload))
	if err != nil {
		t.Errorf("Got unexpected err for URL %s: %v", url, err)
	}
	return r
}

func TestOtherRunnersRESTApi(t *testing.T) {
	iPort := fgrpc.PingServerTCP("0", "", "", "bar", 0)
	iDest := fmt.Sprintf("localhost:%d", iPort)

	mux, addr := fhttp.DynamicHTTPServer(false)
	AddHandlers(mux, "/fortio/", "/tmp")
	restURL := fmt.Sprintf("http://localhost:%d/fortio/jrpc/run", addr.Port)

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
