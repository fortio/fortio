// Copyright 2020 Fortio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package fhttp

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"
)

func TestMultiProxy(t *testing.T) {
	_, debugAddr := ServeTCP("0", "/debug")
	urlBase := fmt.Sprintf("localhost:%d/", debugAddr.Port)
	for i := 0; i < 2; i++ {
		serial := (i == 0)
		mcfg := MultiServerConfig{Serial: serial}
		mcfg.Targets = []TargetConf{{Destination: urlBase, MirrorOrigin: true},
			{Destination: urlBase + "debug", MirrorOrigin: false},
			{Destination: urlBase + "echo?status=555"}}
		_, multiAddr := MultiServer("0", &mcfg)
		url := fmt.Sprintf("http://%s/debug", multiAddr)
		payload := "A test payload"
		opts := HTTPOptions{URL: url, Payload: []byte(payload)}
		opts.AddAndValidateExtraHeader("b3: traceid...")
		opts.AddAndValidateExtraHeader("X-FA: bar") // so it comes just before X-Fortio-Multi-Id
		code, data := Fetch(&opts)
		if serial && code != http.StatusOK {
			t.Errorf("Got %d %s instead of ok in serial mode (first response sets code) for %s", code, DebugSummary(data, 256), url)
		}
		if !serial && code != 555 {
			t.Errorf("Got %d %s instead of 555 in parallel mode (non ok response sets code) for %s", code, DebugSummary(data, 256), url)
		}
		if !bytes.Contains(data, []byte(payload)) {
			t.Errorf("Missing expected payload %q in %s", payload, DebugSummary(data, 1024))
		}
		searchFor := "B3: traceid..."
		if !bytes.Contains(data, []byte(searchFor)) {
			t.Errorf("Missing expected trace header %q in %s", searchFor, DebugSummary(data, 1024))
		}
		searchFor = "\nX-Fa: bar\nX-Fortio-Multi-Id: 1\n"
		if !bytes.Contains(data, []byte(searchFor)) {
			t.Errorf("Missing expected general header %q in 1st req %s", searchFor, DebugSummary(data, 1024))
		}
		searchFor = "\nX-Fa: bar\nX-Fortio-Multi-Id: 2\n"
		if bytes.Contains(data, []byte(searchFor)) {
			t.Errorf("Unexpected non trace header %q in 2nd req %s", searchFor, DebugSummary(data, 1024))
		}
		// Issue #624
		if bytes.Contains(data, []byte("gzip")) {
			t.Errorf("Unexpected gzip (accept encoding)in %s", DebugSummary(data, 1024))
		}
		searchFor = "X-Fortio-Multi-Id: 1"
		if !bytes.Contains(data, []byte(searchFor)) {
			t.Errorf("Missing expected %q in %s", searchFor, DebugSummary(data, 1024))
		}
		// Second request should be found
		searchFor = "X-Fortio-Multi-Id: 2"
		if !bytes.Contains(data, []byte(searchFor)) {
			t.Errorf("Missing expected %q in %s", searchFor, DebugSummary(data, 1024))
		}
		// Third request errors 100% so shouldn't be found
		searchFor = "X-Fortio-Multi-Id: 3"
		if bytes.Contains(data, []byte(searchFor)) {
			t.Errorf("Unexpected %q in %s", searchFor, DebugSummary(data, 1024))
		}
	}
}

func TestMultiProxyErrors(t *testing.T) {
	for i := 0; i < 2; i++ {
		serial := (i == 0)
		mcfg := MultiServerConfig{Serial: serial}
		// No scheme in url to cause error
		mcfg.Targets = []TargetConf{
			{Destination: "\001doesntexist.fortio.org:2435/foo"},
			{Destination: "\001doesntexist.fortio.org:2435/foo", MirrorOrigin: true},
			{Destination: "doesntexist.fortio.org:2435/foo"},
		}
		_, multiAddr := MultiServer("0", &mcfg)
		url := fmt.Sprintf("http://%s/debug", multiAddr)
		opts := HTTPOptions{URL: url}
		code, data := Fetch(&opts)
		if code != http.StatusServiceUnavailable {
			t.Errorf("Got %d %s instead of StatusServiceUnavailable for %s", code, DebugSummary(data, 256), url)
		}
	}
}

// -- end of benchmark tests / end of this file
