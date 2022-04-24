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

package fhttp

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"net/http"
	"os"
	"path"
	"runtime"
	"strings"
	"testing"
	"time"

	"fortio.org/fortio/log"
)

func TestHTTPRunner(t *testing.T) {
	mux, addr := DynamicHTTPServer(false)
	mux.HandleFunc("/foo/", EchoHandler)
	baseURL := fmt.Sprintf("http://localhost:%d/", addr.Port)

	opts := HTTPRunnerOptions{}
	opts.QPS = 100
	opts.URL = baseURL
	opts.DisableFastClient = true
	_, err := RunHTTPTest(&opts)
	if err == nil {
		t.Error("Expecting an error but didn't get it when not using full url")
	}
	opts.DisableFastClient = false
	opts.URL = baseURL + "foo/bar?delay=20ms&status=200:100"
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
	if res.SocketCount != res.RunnerResults.NumThreads {
		t.Errorf("%d socket used, expected same as thread# %d", res.SocketCount, res.RunnerResults.NumThreads)
	}
	// Test raw client, should get warning about non init timeout:
	rawOpts := HTTPOptions{
		URL: opts.URL,
	}
	o1 := rawOpts
	fc, _ := NewFastClient(&o1)
	if r, _, _ := fc.Fetch(); r != http.StatusOK {
		t.Errorf("Fast Client with raw option should still work with warning in logs")
	}
	o1 = rawOpts
	o1.URL = "http://www.doesnotexist.badtld/"
	c, _ := NewStdClient(&o1)
	c.ChangeURL(rawOpts.URL)
	if r, _, _ := c.Fetch(); r != http.StatusOK {
		t.Errorf("Std Client with raw option should still work with warning in logs")
	}
}

func testHTTPNotLeaking(t *testing.T, opts *HTTPRunnerOptions) {
	ngBefore1 := runtime.NumGoroutine()
	t.Logf("Number go routine before test %d", ngBefore1)
	mux, addr := DynamicHTTPServer(false)
	mux.HandleFunc("/echo100", EchoHandler)
	// Avoid using localhost which can timeout with stdclient (thought this might fail on ipv6 only machines?)
	url := fmt.Sprintf("http://127.0.0.1:%d/echo100", addr.Port)
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
	t.Logf("Number of go routine after warm up / before 2nd test %d", ngBefore2)
	// 2nd run, should be stable number of go routines after first, not keep growing:
	res, err = RunHTTPTest(opts)
	// it takes a while for the connections to close with std client (!) why isn't CloseIdleConnections() synchronous
	runtime.GC()
	runtime.GC() // 2x to clean up more... (#178)
	ngAfter := runtime.NumGoroutine()
	t.Logf("Number of go routine after 2nd test %d", ngAfter)
	if err != nil {
		t.Error(err)
		return
	}
	httpOk = res.RetCodes[http.StatusOK]
	if opts.Exactly != httpOk {
		t.Errorf("Run2: Mismatch between requested calls %d and ok %v", numCalls, res.RetCodes)
	}
	// allow for ~8 goroutine variance, as we use 50 if we leak it will show (was failing before #167)
	if ngAfter > ngBefore2+8 {
		t.Errorf("Goroutines after test %d, expected it to stay near %d", ngAfter, ngBefore2)
	}
	if !opts.DisableFastClient {
		// only fast client so far has a socket count
		if res.SocketCount != res.RunnerResults.NumThreads {
			t.Errorf("%d socket used, expected same as thread# %d", res.SocketCount, res.RunnerResults.NumThreads)
		}
	}
}

func TestHttpNotLeakingFastClient(t *testing.T) {
	testHTTPNotLeaking(t, &HTTPRunnerOptions{})
}

func TestHttpNotLeakingStdClient(t *testing.T) {
	testHTTPNotLeaking(t, &HTTPRunnerOptions{HTTPOptions: HTTPOptions{DisableFastClient: true}})
}

func testPayloadWarmRace(t *testing.T, o *HTTPRunnerOptions) {
	mux, addr := DynamicHTTPServer(false)
	mux.HandleFunc("/echo123/", EchoHandler)
	URL := fmt.Sprintf("http://localhost:%d/echo123/", addr.Port)
	o.Init(URL)
	o.NumConnections = 4
	o.QPS = 16
	o.Duration = 2 * time.Second
	o.Payload = []byte("abc")
	res, err := RunHTTPTest(o)
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

func TestPayloadWarmRaceStdClient(t *testing.T) {
	testPayloadWarmRace(t, &HTTPRunnerOptions{HTTPOptions: HTTPOptions{DisableFastClient: true}})
	testPayloadWarmRace(t, &HTTPRunnerOptions{HTTPOptions: HTTPOptions{DisableFastClient: true, SequentialWarmup: true}})
}

func TestPayloadWarmRaceFastClient(t *testing.T) {
	testPayloadWarmRace(t, &HTTPRunnerOptions{})
	testPayloadWarmRace(t, &HTTPRunnerOptions{HTTPOptions: HTTPOptions{SequentialWarmup: true}})
}

func TestHTTPRunnerClientRace(t *testing.T) {
	mux, addr := DynamicHTTPServer(false)
	mux.HandleFunc("/echo1/", EchoHandler)
	URL := fmt.Sprintf("http://localhost:%d/echo1/", addr.Port)

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

func TestClosingAndSocketCount(t *testing.T) {
	mux, addr := DynamicHTTPServer(false)
	mux.HandleFunc("/echo42/", EchoHandler)
	URL := fmt.Sprintf("http://localhost:%d/echo42/?close=true", addr.Port)
	opts := HTTPRunnerOptions{}
	opts.Init(URL)
	opts.QPS = 10
	numReq := int64(50) // can't do too many without running out of fds on mac
	opts.Exactly = numReq
	opts.NumThreads = 5
	res, err := RunHTTPTest(&opts)
	if err != nil {
		t.Fatal(err)
	}
	totalReq := res.DurationHistogram.Count
	if totalReq != numReq {
		t.Errorf("Mismatch between requests %d and expected %d", totalReq, numReq)
	}
	httpOk := res.RetCodes[http.StatusOK]
	if totalReq != httpOk {
		t.Errorf("Mismatch between requests %d and ok %v", totalReq, res.RetCodes)
	}
	if int64(res.SocketCount) != numReq {
		t.Errorf("When closing, got %d while expected as many sockets as requests %d", res.SocketCount, numReq)
	}
}

func TestHTTPRunnerBadServer(t *testing.T) {
	// Using http to an https server (or the current 'close all' dummy https server)
	// should fail:
	_, addr := DynamicHTTPServer(true)
	baseURL := fmt.Sprintf("http://localhost:%d/", addr.Port)

	opts := HTTPRunnerOptions{}
	opts.QPS = 10
	opts.Init(baseURL)
	_, err := RunHTTPTest(&opts)
	if err == nil {
		t.Fatal("Expecting an error but didn't get it when connecting to bad server")
	}
	log.Infof("Got expected error from mismatch/bad server: %v", err)
}

func gUnzipData(t *testing.T, data []byte) (resData []byte) {
	b := bytes.NewBuffer(data)

	r, err := gzip.NewReader(b)
	if err != nil {
		t.Errorf("gunzip NewReader: %v", err)
		return
	}
	var resB bytes.Buffer
	_, err = resB.ReadFrom(r)
	if err != nil {
		t.Errorf("gunzip ReadFrom: %v", err)
		return
	}
	resData = resB.Bytes()
	return
}

// nolint: gocognit
func TestAccessLog(t *testing.T) {
	mux, addr := DynamicHTTPServer(false)
	mux.HandleFunc("/echo-for-alog/", EchoHandler)
	URL := fmt.Sprintf("http://localhost:%d/echo-for-alog/?status=555:50", addr.Port)
	opts := HTTPRunnerOptions{}
	opts.Init(URL)
	opts.QPS = 10
	numReq := int64(50) // can't do too many without running out of fds on mac
	opts.Exactly = numReq
	opts.NumThreads = 5
	for _, format := range []string{"json", "influx"} {
		dir := t.TempDir()
		fname := path.Join(dir, "access.log")
		err := opts.AddAccessLogger(fname, format)
		if err != nil {
			t.Errorf("unexpected error for log file %q %s: %v", fname, format, err)
		}
		res, err := RunHTTPTest(&opts)
		if err != nil {
			t.Fatal(err)
		}
		totalReq := res.DurationHistogram.Count
		if totalReq != numReq {
			t.Errorf("Mismatch between requests %d and expected %d", totalReq, numReq)
		}
		httpOk := res.RetCodes[http.StatusOK]
		http555 := res.RetCodes[555]
		if httpOk <= 1 || httpOk >= numReq-1 {
			t.Errorf("Unexpected ok count %d should be ~ 50%% of %d", httpOk, numReq)
		}
		if http555 <= 1 || http555 >= numReq-1 {
			t.Errorf("Unexpected 555 count %d should be ~ 50%% of %d", http555, numReq)
		}
		if totalReq != httpOk+http555 {
			t.Errorf("Mismatch between requests %d and ok+555 %v", totalReq, res.RetCodes)
		}
		file, _ := os.Open(fname)
		scanner := bufio.NewScanner(file)
		lineCount := 0
		linesOk := 0
		linesNotOk := 0
		lines200 := 0
		lines555 := 0
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "true") {
				linesOk++
			}
			if strings.Contains(line, "false") {
				linesNotOk++
			}
			if strings.Contains(line, "\"200\"") {
				lines200++
			}
			if strings.Contains(line, "\"555\"") {
				lines555++
			}
			lineCount++
		}
		if lineCount != int(numReq) {
			t.Errorf("unexpected number of lines in access log %s: %d", format, lineCount)
		}
		if linesOk != int(httpOk) {
			t.Errorf("unexpected number of lines in access log %s: with ok: %d instead of %d", format, linesOk, httpOk)
		}
		if lines200 != int(httpOk) {
			t.Errorf("unexpected number of lines in access log %s: with 200: %d instead of %d", format, lines200, httpOk)
		}
		if linesNotOk != int(http555) {
			t.Errorf("unexpected number of lines in access log %s: with not ok: %d instead of %d", format, linesNotOk, http555)
		}
		if lines555 != int(http555) {
			t.Errorf("unexpected number of lines in access log %s: with 555: %d instead of %d", format, lines555, http555)
		}
	}
}

// need to be the last test as it installs Serve() which would make
// the error test for / url above fail:

func TestServe(t *testing.T) {
	_, addr := ServeTCP("0", "/debugx1")
	port := addr.Port
	log.Infof("On addr %s found port: %d", addr, port)
	if port == 0 {
		t.Errorf("outport: %d must be different", port)
	}
	url := fmt.Sprintf("http://localhost:%d/debugx1?env=dump", port)
	time.Sleep(100 * time.Millisecond)
	o := NewHTTPOptions(url)
	o.AddAndValidateExtraHeader("X-Header: value1")
	o.AddAndValidateExtraHeader("X-Header: value2")
	c, _ := NewClient(o)
	code, data, _ := c.Fetch()
	if code != http.StatusOK {
		t.Errorf("Unexpected non 200 ret code for debug url %s : %d", url, code)
	}
	if len(data) <= 100 {
		t.Errorf("Unexpected short data for debug url %s : %s", url, DebugSummary(data, 101))
	}
	if !strings.Contains(string(data), "X-Header: value1,value2") {
		t.Errorf("Multi header not found in %s", DebugSummary(data, 1024))
	}
	url2 := fmt.Sprintf("http://localhost:%d/debugx1/echo/foo", port)
	o2 := NewHTTPOptions(url2)
	o2.Payload = []byte("abcd")
	c2, _ := NewClient(o2)
	code2, data2, header := c2.Fetch()
	if code2 != http.StatusOK {
		t.Errorf("Unexpected non 200 ret code for debug echo url %s : %d", url2, code2)
	}
	if string(data2[header:]) != "abcd" {
		t.Errorf("Unexpected that %s isn't an echo server, got %q", url2, string(data2))
	}
	// Accept gzip but no actual gzip=true
	o2.AddAndValidateExtraHeader("Accept-Encoding: gzip")
	c3, _ := NewClient(o2)
	code3, data3, header := c3.Fetch()
	if code3 != http.StatusOK {
		t.Errorf("Unexpected non 200 ret code for debug echo url %s : %d", url2, code3)
	}
	if string(data3[header:]) != "abcd" {
		t.Errorf("Unexpected that %s with Accept-Encoding: gzip but no gzip true isn't a plain echo server, got %q", url2, string(data3))
	}
	url4 := url2 + "?gzip=true"
	o4 := NewHTTPOptions(url4)
	expected4 := "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz"
	o4.Payload = []byte(expected4)
	o4.AddAndValidateExtraHeader("Accept-Encoding: gzip")
	c4, _ := NewClient(o4)
	code4, data4, header := c4.Fetch()
	if code4 != http.StatusOK {
		t.Errorf("Unexpected non 200 ret code for debug echo gziped url %s : %d", url4, code4)
	}
	if string(data4[header:]) == expected4 {
		t.Errorf("Unexpected that %s with Accept-Encoding: gzip and ?gzip=true is a plain echo server, got %q", url4, string(data4))
	}
	if len(data4)-header >= len(expected4) {
		t.Errorf("Unexpected that %s with Accept-Encoding: gzip and ?gzip=true returns bigger payload %d (not compressed), got %q",
			url4, len(data4)-header, string(data4))
	}
	data4unzip := gUnzipData(t, data4[header:])
	if string(data4unzip) != expected4 {
		t.Errorf("Unexpected that %s with Accept-Encoding: gzip and ?gzip=true doesn't gunzip to echo, got %q", url4, string(data4))
	}
	url5 := url4 + "&size=400"
	o5 := NewHTTPOptions(url5)
	c5, _ := NewClient(o5)
	code5, data5, header := c5.Fetch()
	if code5 != http.StatusOK {
		t.Errorf("Unexpected non 200 ret code for debug echo gziped url %s : %d", url5, code5)
	}
	expected6 := data5[header:] // when we actually compress we should get same as this after gunzip
	if len(data5)-header != 400 {
		t.Errorf("Unexpected that %s without Accept-Encoding: gzip and ?gzip=true&size=400 should return 400 bytes, got %d",
			url5, len(data5)-header)
	}
	o5.AddAndValidateExtraHeader("Accept-Encoding: gzip")
	c6, _ := NewClient(o5)
	code6, data6, header := c6.Fetch()
	if code6 != http.StatusOK {
		t.Errorf("Unexpected non 200 ret code for debug echo gziped url %s : %d", url5, code6)
	}
	data6unzip := gUnzipData(t, data6[header:])
	if !bytes.Equal(data6unzip, expected6) {
		t.Errorf("Unexpected that %s with Accept-Encoding: gzip and ?gzip=true doesn't gunzip to echo, got %q", url5, string(data6))
	}
	if len(data6)-header <= 400 {
		t.Errorf("Unexpected that %s with Accept-Encoding: gzip and ?gzip=true and random payload compresses to lower than 400: %d",
			url5, len(data6)-header)
	}
}

func TestAbortOn(t *testing.T) {
	mux, addr := DynamicHTTPServer(false)
	mux.HandleFunc("/foo/", EchoHandler)
	baseURL := fmt.Sprintf("http://localhost:%d/", addr.Port)
	o := HTTPRunnerOptions{}
	o.URL = baseURL
	o.AbortOn = 404
	o.Exactly = 40
	o.NumThreads = 4
	o.QPS = 10
	r, err := RunHTTPTest(&o)
	if err != nil {
		t.Errorf("Error while starting runner1: %v", err)
	}
	count := r.Result().DurationHistogram.Count
	if count > int64(o.NumThreads) {
		t.Errorf("Abort1 not working, did %d requests expecting ideally 1 and <= %d", count, o.NumThreads)
	}
	o.URL += "foo/"
	r, err = RunHTTPTest(&o)
	if err != nil {
		t.Errorf("Error while starting runner2: %v", err)
	}
	count = r.Result().DurationHistogram.Count
	if count != o.Exactly {
		t.Errorf("Did %d requests when expecting all %d (non matching AbortOn)", count, o.Exactly)
	}
	o.AbortOn = 200
	r, err = RunHTTPTest(&o)
	if err != nil {
		t.Errorf("Error while starting runner3: %v", err)
	}
	count = r.Result().DurationHistogram.Count
	if count > int64(o.NumThreads) {
		t.Errorf("Abort2 not working, did %d requests expecting ideally 1 and <= %d", count, o.NumThreads)
	}
}
