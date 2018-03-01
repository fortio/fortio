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

package fhttp

import (
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"testing"
	"time"

	"istio.io/fortio/log"
)

func TestHTTPRunner(t *testing.T) {
	port, mux := DynamicHTTPServer(false)
	mux.HandleFunc("/foo/", EchoHandler)
	baseURL := fmt.Sprintf("http://localhost:%d/", port)

	opts := HTTPRunnerOptions{}
	opts.QPS = 100
	opts.URL = baseURL
	opts.DisableFastClient = true
	_, err := RunHTTPTest(&opts)
	if err == nil {
		t.Error("Expecting an error but didn't get it when not using full url")
	}
	opts.DisableFastClient = false
	opts.URL = baseURL + "foo/bar?delay=2s&status=200:100"
	opts.Profiler = "test.profile"
	res, err := RunHTTPTest(&opts)
	if err != nil {
		t.Error(err)
		return
	}
	totalReq := res.DurationHistogram.Count
	httpOk := res.RetCodes[http.StatusOK]
	if totalReq != httpOk {
		t.Errorf("Mismatch between requests %d and ok %v", totalReq, res.RetCodes)
	}
	// Test raw client, should get warning about non init timeout:
	rawOpts := HTTPOptions{
		URL: opts.URL,
	}
	o1 := rawOpts
	if r, _, _ := NewFastClient(&o1).Fetch(); r != http.StatusOK {
		t.Errorf("Fast Client with raw option should still work with warning in logs")
	}
	o1 = rawOpts
	o1.URL = "http://www.doesnotexist.badtld/"
	c := NewStdClient(&o1)
	c.ChangeURL(rawOpts.URL)
	if r, _, _ := c.Fetch(); r != http.StatusOK {
		t.Errorf("Std Client with raw option should still work with warning in logs")
	}
}

func testHTTPNotLeaking(t *testing.T, opts *HTTPRunnerOptions) {
	ngBefore1 := runtime.NumGoroutine()
	t.Logf("Number go rountine before test %d", ngBefore1)
	port, mux := DynamicHTTPServer(false)
	mux.HandleFunc("/echo100", EchoHandler)
	url := fmt.Sprintf("http://localhost:%d/echo100", port)
	numCalls := 100
	opts.NumThreads = numCalls / 2 // make 2 calls per thread
	opts.Exactly = int64(numCalls)
	opts.QPS = float64(numCalls) / 2 // take 1 second
	opts.URL = url
	// Warm up round 1
	res, err := RunHTTPTest(opts)
	if err != nil {
		t.Error(err)
		return
	}
	httpOk := res.RetCodes[http.StatusOK]
	if opts.Exactly != httpOk {
		t.Errorf("Run1: Mismatch between requested calls %d and ok %v", numCalls, res.RetCodes)
	}
	ngBefore2 := runtime.NumGoroutine()
	t.Logf("Number go rountine after warm up / before 2nd test %d", ngBefore2)
	// 2nd run, should be stable number of go routines after first, not keep growing:
	res, err = RunHTTPTest(opts)
	// it takes a while for the connections to close with std client (!) why isn't CloseIdleConnections() synchronous
	runtime.GC()
	ngAfter := runtime.NumGoroutine()
	t.Logf("Number go rountine after 2nd test %d", ngAfter)
	if err != nil {
		t.Error(err)
		return
	}
	httpOk = res.RetCodes[http.StatusOK]
	if opts.Exactly != httpOk {
		t.Errorf("Run2: Mismatch between requested calls %d and ok %v", numCalls, res.RetCodes)
	}
	// allow for ~5 goroutine variance, as we use 50 if we leak it will show
	if ngAfter > ngBefore2+5 {
		t.Errorf("Goroutines after test %d, expected it to stay near %d", ngAfter, ngBefore2)
	}
}

func TestHttpNotLeakingFastClient(t *testing.T) {
	testHTTPNotLeaking(t, &HTTPRunnerOptions{})
}

func TestHttpNotLeakingStdClient(t *testing.T) {
	testHTTPNotLeaking(t, &HTTPRunnerOptions{HTTPOptions: HTTPOptions{DisableFastClient: true}})
}

func TestHTTPRunnerClientRace(t *testing.T) {
	port, mux := DynamicHTTPServer(false)
	mux.HandleFunc("/echo1/", EchoHandler)
	URL := fmt.Sprintf("http://localhost:%d/echo1/", port)

	opts := HTTPRunnerOptions{}
	opts.Init(URL)
	opts.QPS = 100
	opts2 := opts
	go RunHTTPTest(&opts2)
	res, err := RunHTTPTest(&opts)
	if err != nil {
		t.Error(err)
		return
	}
	totalReq := res.DurationHistogram.Count
	httpOk := res.RetCodes[http.StatusOK]
	if totalReq != httpOk {
		t.Errorf("Mismatch between requests %d and ok %v", totalReq, res.RetCodes)
	}
}

func TestHTTPRunnerBadServer(t *testing.T) {
	// Using http to an https server (or the current 'close all' dummy https server)
	// should fail:
	port, _ := DynamicHTTPServer(true)
	baseURL := fmt.Sprintf("http://localhost:%d/", port)

	opts := HTTPRunnerOptions{}
	opts.QPS = 10
	opts.Init(baseURL)
	_, err := RunHTTPTest(&opts)
	if err == nil {
		t.Fatal("Expecting an error but didn't get it when connecting to bad server")
	}
	log.Infof("Got expected error from mismatch/bad server: %v", err)
}

// need to be the last test as it installs Serve() which would make
// the error test for / url above fail:

func TestServe(t *testing.T) {
	addr := Serve("0", "/debugx1")
	port := addr.Port
	log.Infof("On addr %s found port: %d", addr, port)
	url := fmt.Sprintf("http://localhost:%d/debugx1?env=dump", port)
	if port == 0 {
		t.Errorf("outport: %d must be different", port)
	}
	time.Sleep(100 * time.Millisecond)
	o := NewHTTPOptions(url)
	o.AddAndValidateExtraHeader("X-Header: value1")
	o.AddAndValidateExtraHeader("X-Header: value2")
	code, data, _ := NewClient(o).Fetch()
	if code != http.StatusOK {
		t.Errorf("Unexpected non 200 ret code for debug url %s : %d", url, code)
	}
	if len(data) <= 100 {
		t.Errorf("Unexpected short data for debug url %s : %s", url, DebugSummary(data, 101))
	}
	if !strings.Contains(string(data), "X-Header: value1,value2") {
		t.Errorf("Multi header not found in %s", DebugSummary(data, 1024))
	}
}
