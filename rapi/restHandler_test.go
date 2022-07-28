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
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"fortio.org/fortio/fhttp"
)

func Fetch(url string, jsonPayload string) (int, []byte) {
	opts := fhttp.NewHTTPOptions(url)
	opts.DisableFastClient = true      // not get raw/chunked results
	opts.Payload = []byte(jsonPayload) // Will make a POST if not empty
	opts.HTTPReqTimeOut = 10 * time.Second
	return fhttp.Fetch(opts)
}

// If jsonPayload isn't empty we POST otherwise get the url.
func GetResult(t *testing.T, url string, jsonPayload string) (*fhttp.HTTPRunnerResults, []byte) {
	code, bytes := Fetch(url, jsonPayload)
	if code != http.StatusOK {
		t.Errorf("Got unexpected error code: URL %s: %v - %s", url, code, fhttp.DebugSummary(bytes, 512))
	}
	res := fhttp.HTTPRunnerResults{}
	err := json.Unmarshal(bytes, &res)
	if err != nil {
		t.Fatalf("Unable to deserialize results: %q: %v", string(bytes), err)
	}
	return &res, bytes
}

// Same as above but when expecting to get an error reply.
func GetErrorResult(t *testing.T, url string, jsonPayload string) (*ErrorReply, []byte) {
	code, bytes := Fetch(url, jsonPayload)
	if code == http.StatusOK {
		t.Errorf("Got unexpected ok code: URL %s: %v", url, code)
	}
	res := ErrorReply{}
	err := json.Unmarshal(bytes, &res)
	if err != nil {
		t.Fatalf("Unable to deserialize error reply: %q: %v", string(bytes), err)
	}
	return &res, bytes
}

// Same as above but when expecting to get an Async reply.
func GetAsyncResult(t *testing.T, url string, jsonPayload string) (*AsyncReply, []byte) {
	code, bytes := Fetch(url, jsonPayload)
	if code != http.StatusOK {
		t.Errorf("Got unexpected error code: URL %s: %v", url, code)
	}
	res := AsyncReply{}
	err := json.Unmarshal(bytes, &res)
	if err != nil {
		t.Fatalf("Unable to deserialize async reply: %q: %v", string(bytes), err)
	}
	return &res, bytes
}

// nolint: funlen // it's a test of a lot of things in sequence/context
func TestRestHTTPRunnerRESTApi(t *testing.T) {
	mux, addr := fhttp.DynamicHTTPServer(false)
	mux.HandleFunc("/foo/", fhttp.EchoHandler)
	baseURL := fmt.Sprintf("http://localhost:%d/", addr.Port)
	uiPath := "/fortio/"
	tmpDir := t.TempDir()
	os.Create(path.Join(tmpDir, "foo.txt")) // not a json, will be skipped over
	badJson := path.Join(tmpDir, "bad.json")
	os.Create(badJson)
	os.Chmod(badJson, 0) // make the file un readable so it should also be skipped
	AddHandlers(mux, uiPath, tmpDir)
	mux.HandleFunc("/data/index.tsv", func(w http.ResponseWriter, r *http.Request) { SendTSVDataIndex("/data/", w) })

	restURL := fmt.Sprintf("http://localhost:%d%s%s", addr.Port, uiPath, restRunURI)

	runURL := fmt.Sprintf("%s?qps=%d&url=%s&t=2s", restURL, 100, baseURL)

	res, bytes := GetResult(t, runURL, "")
	if res.RetCodes[200] != 0 {
		t.Errorf("Got unexpected 200s %d on base: %+v - got %s", res.RetCodes[200], res, fhttp.DebugSummary(bytes, 512))
	}
	if res.RetCodes[404] != 2*100 { // 2s at 100qps == 200
		t.Errorf("Got unexpected 404s count %d on base: %+v", res.RetCodes[404], res)
	}
	echoURL := baseURL + "foo/bar?delay=20ms&status=200:100"
	runURL = fmt.Sprintf("%s?qps=%d&url=%s&n=200", restURL, 100, echoURL)
	res, bytes = GetResult(t, runURL, "")
	totalReq := res.DurationHistogram.Count
	httpOk := res.RetCodes[http.StatusOK]
	if totalReq != httpOk {
		t.Errorf("Mismatch between requests %d and ok %v (%+v) - got %s", totalReq, res.RetCodes, res, fhttp.DebugSummary(bytes, 512))
	}
	if res.SocketCount != res.RunnerResults.NumThreads {
		t.Errorf("%d socket used, expected same as thread# %d", res.SocketCount, res.RunnerResults.NumThreads)
	}

	// Check payload is used and that query arg overrides payload
	jsonData := fmt.Sprintf("{\"metadata\": {\"url\":%q, \"save\":\"on\", \"n\":\"200\"}}", echoURL)
	runURL = fmt.Sprintf("%s?jsonPath=.metadata&qps=100&n=100", restURL)
	res, bytes = GetResult(t, runURL, jsonData)
	totalReq = res.DurationHistogram.Count
	httpOk = res.RetCodes[http.StatusOK]
	if totalReq != httpOk {
		t.Errorf("Mismatch between requests %d and ok %v (%+v) - got %s", totalReq, res.RetCodes, res, fhttp.DebugSummary(bytes, 512))
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
	errObj, bytes := GetErrorResult(t, runURL, jsonData)
	if errObj.Error != "parsing duration '42'" || errObj.Exception != "time: missing unit in duration \"42\"" {
		t.Errorf("Didn't get the expected duration parsing error, got %+v - %s", errObj, fhttp.DebugSummary(bytes, 512))
	}
	// bad json path: doesn't exist
	runURL = fmt.Sprintf("%s?jsonPath=.foo", restURL)
	errObj, bytes = GetErrorResult(t, runURL, jsonData)
	if errObj.Exception != "\"foo\" not found in json" {
		t.Errorf("Didn't get the expected json body access error, got %+v - %s", errObj, fhttp.DebugSummary(bytes, 512))
	}
	// bad json path: wrong type
	runURL = fmt.Sprintf("%s?jsonPath=.metadata.url", restURL)
	errObj, bytes = GetErrorResult(t, runURL, jsonData)
	if errObj.Exception != "\"url\" path is not a map" {
		t.Errorf("Didn't get the expected json type mismatch error, got %+v - %s", errObj, fhttp.DebugSummary(bytes, 512))
	}
	// missing url and a few other cases
	jsonData = `{"metadata": {"n": 200}}`
	runURL = fmt.Sprintf("%s?jsonPath=.metadata", restURL)
	errObj, bytes = GetErrorResult(t, runURL, jsonData)
	if errObj.Error != "URL is required" {
		t.Errorf("Didn't get the expected url missing error, got %+v - %s", errObj, fhttp.DebugSummary(bytes, 512))
	}
	// not well formed json
	jsonData = `{"metadata": {"n":`
	runURL = fmt.Sprintf("%s?jsonPath=.metadata", restURL)
	errObj, bytes = GetErrorResult(t, runURL, jsonData)
	if errObj.Exception != "unexpected end of JSON input" {
		t.Errorf("Didn't get the expected error for truncated/invalid json, got %+v - %s", errObj, fhttp.DebugSummary(bytes, 512))
	}
	// Exercise Hearders code (but hard to test the effect,
	// would need to make a single echo query instead of a run... which the API doesn't do)
	jsonData = `{"metadata": {"headers": ["Foo: Bar", "Blah: BlahV"]}}`
	runURL = fmt.Sprintf("%s?jsonPath=.metadata&qps=90&n=23&url=%s&H=Third:HeaderV", restURL, echoURL)
	res, bytes = GetResult(t, runURL, jsonData)
	if res.RetCodes[http.StatusOK] != 23 {
		t.Errorf("Should have done expected 23 requests, got %+v - %s", res.RetCodes, fhttp.DebugSummary(bytes, 128))
	}
	// Start infinite running run
	runURL = fmt.Sprintf("%s?jsonPath=.metadata&qps=10&t=on&url=%s&async=on", restURL, echoURL)
	asyncObj, bytes := GetAsyncResult(t, runURL, jsonData)
	runID := asyncObj.RunID
	if asyncObj.Message != "started" || runID <= savedID {
		t.Errorf("Should started async job got %+v - %s", asyncObj, fhttp.DebugSummary(bytes, 256))
	}
	// And stop it:
	stopURL := fmt.Sprintf("http://localhost:%d%s%s?runid=%d", addr.Port, uiPath, restStopURI, runID)
	asyncObj, bytes = GetAsyncResult(t, stopURL, "")
	stoppedMsg := "stopped"
	if asyncObj.Message != stoppedMsg || asyncObj.RunID != runID || asyncObj.Count != 1 {
		t.Errorf("Should have stopped async job got %+v - %s", asyncObj, fhttp.DebugSummary(bytes, 256))
	}
	// Stop it again, should be 0 count
	asyncObj, bytes = GetAsyncResult(t, stopURL, "")
	if asyncObj.Message != stoppedMsg || asyncObj.RunID != runID || asyncObj.Count != 0 {
		t.Errorf("2nd stop should be noop, got %+v - %s", asyncObj, fhttp.DebugSummary(bytes, 256))
	}
	// Start 3 async test and stop all
	runURL = fmt.Sprintf("%s?jsonPath=.metadata&qps=1&t=on&url=%s&async=on", restURL, echoURL)
	_, _ = GetAsyncResult(t, runURL, jsonData)
	_, _ = GetAsyncResult(t, runURL, jsonData)
	_, _ = GetAsyncResult(t, runURL, jsonData)
	stopURL = fmt.Sprintf("http://localhost:%d%s%s", addr.Port, uiPath, restStopURI)
	asyncObj, bytes = GetAsyncResult(t, stopURL, "")
	if asyncObj.Message != stoppedMsg || asyncObj.RunID != 0 || asyncObj.Count != 3 {
		t.Errorf("Should have stopped 3 async job got %+v - %s", asyncObj, fhttp.DebugSummary(bytes, 256))
	}

	tsvURL := fmt.Sprintf("http://localhost:%d%s", addr.Port, "/data/index.tsv")
	code, bytes := fhttp.FetchURL(tsvURL)
	if code != http.StatusOK {
		t.Errorf("Error getting tsv index: %d - got %s", code, fhttp.DebugSummary(bytes, 512))
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
	if strings.Contains(str, "bad.json") {
		t.Errorf("Result of index.tsv should not include unreadble .json files: %s", str)
	}
	files := DataList()
	if len(files) < 1 {
		t.Error("DataList() should also return files when dir is correct")
	}
	SetDataDir("/does/not/exist")
	code, bytes = fhttp.FetchURL(tsvURL)
	if code != http.StatusServiceUnavailable {
		t.Errorf("Setting bad directory should error out, it didn't - got %s", fhttp.DebugSummary(bytes, 512))
	}
	none := DataList()
	if len(none) > 0 {
		t.Errorf("Setting bad directory should not get any files got %v", none)
	}
}
