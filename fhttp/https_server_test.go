// Copyright 2023 Fortio Authors.
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
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fortio.org/fortio/fnet"
	"fortio.org/fortio/log"
)

var (
	// Generated from "make cert".
	caCrt  = "../cert-tmp/ca.crt"
	svrCrt = "../cert-tmp/server.crt"
	svrKey = "../cert-tmp/server.key"
)

func TestHTTPSServer(t *testing.T) {
	// log.SetLogLevel(log.Debug)
	m, a := ServeTLS("0", "/debug", svrCrt, svrKey)
	if m == nil || a == nil {
		t.Errorf("Failed to create server %v %v", m, a)
	}
	url := fmt.Sprintf("https://localhost:%d/debug", a.(*net.TCPAddr).Port)
	// Triggers the h2->DisableFastClient normalization code too.
	o := HTTPOptions{URL: url, TLSOptions: TLSOptions{CACert: caCrt}, H2: true}
	client, _ := NewClient(&o)
	code, data, header := client.Fetch(context.Background())
	t.Logf("TestDebugHandlerSortedHeaders result code %d, data len %d, headerlen %d", code, len(data), header)
	if code != http.StatusOK {
		t.Errorf("Got %d instead of 200", code)
	}
	body := string(data)
	if !strings.Contains(body, "https TLS_") {
		t.Errorf("Missing https TLS_ in body: %s", body)
	}
	if !strings.Contains(body, "HTTP/2.0") {
		t.Errorf("Missing HTTP/2.0 in body: %s", body)
	}
}

func TestHTTPSServerError(t *testing.T) {
	_, addr := ServeTLS("0", "", svrCrt, svrKey)
	port := fnet.GetPort(addr)
	mux2, addr2 := ServeTLS(port, "", svrCrt, svrKey)
	if mux2 != nil || addr2 != nil {
		t.Errorf("2nd Serve() on same port %v should have failed: %v %v", port, mux2, addr2)
	}
}

func TestHTTPSServerMissingCert(t *testing.T) {
	fatalCalled := atomic.Bool{}
	fatalCalled.Store(false)
	log.FatalExit = func(int) {
		t.Logf("FatalExit called")
		fatalCalled.Store(true)
	}
	log.SetFlagDefaultsForClientTools()
	_, addr := ServeTLS("0", "", "/foo/bar.crt", "/foo/bar.key")
	url := fmt.Sprintf("https://localhost:%d/debug", addr.(*net.TCPAddr).Port)
	o := HTTPOptions{URL: url, TLSOptions: TLSOptions{CACert: caCrt}, H2: true, HTTPReqTimeOut: 100 * time.Millisecond}
	client, _ := NewClient(&o)
	code, data, header := client.Fetch(context.Background())
	t.Logf("TestDebugHandlerSortedHeaders result code %d, data len %d, headerlen %d", code, len(data), header)
	if code != -1 {
		t.Errorf("Got %d instead of expected error", code)
	}
	if !fatalCalled.Load() {
		t.Errorf("FatalExit not called")
	}
}
