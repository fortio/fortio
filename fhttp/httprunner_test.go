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
	"testing"

	"istio.io/fortio/log"
)

func TestHTTPRunner(t *testing.T) {
	http.HandleFunc("/foo/", EchoHandler)
	port := DynamicHTTPServer(false)
	baseURL := fmt.Sprintf("http://localhost:%d/", port)

	opts := HTTPRunnerOptions{}
	opts.QPS = 100
	opts.Init(baseURL)
	_, err := RunHTTPTest(&opts)
	if err == nil {
		t.Error("Expecting an error but didn't get it when not using full url")
	}
	opts.URL = baseURL + "foo/bar?delay=100us"
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

func TestHTTPRunnerClientRace(t *testing.T) {
	http.HandleFunc("/echo1/", EchoHandler)
	port := DynamicHTTPServer(false)
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
	port := DynamicHTTPServer(true)
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
