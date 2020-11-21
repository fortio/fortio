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

	"fortio.org/fortio/log"
)

func init() {
	log.SetLogLevel(log.Debug)
}

func TestMultiProxy(t *testing.T) {
	_, debugAddr := ServeTCP("0", "/debug")
	urlBase := fmt.Sprintf("localhost:%d/", debugAddr.Port)
	for i := 0; i < 2; i++ {
		serial := (i == 0)
		mcfg := MultiServerConfig{Serial: serial}
		mcfg.Targets = []TargetConf{{Destination: urlBase, MirrorOrigin: true}, {Destination: urlBase + "echo?status=555"}}
		_, multiAddr := MultiServer("0", &mcfg)
		url := fmt.Sprintf("http://%s/debug", multiAddr)
		payload := "A test payload"
		opts := HTTPOptions{URL: url, Payload: []byte(payload)}
		opts.AddAndValidateExtraHeader("b3: traceid...")
		code, data := Fetch(&opts)
		if serial && code != http.StatusOK {
			t.Errorf("Got %d %s instead of ok in serial mode (first response sets code) for %s", code, DebugSummary(data, 256), url)
		}
		if !serial && code != 555 {
			t.Errorf("Got %d %s instead of 555 in parallel mode (non ok response sets code) for %s", code, DebugSummary(data, 256), url)
		}
		if !bytes.Contains(data, []byte(payload)) {
			t.Errorf("Result %s doesn't contain expected payload echo back %q", DebugSummary(data, 1024), payload)
		}
		if !bytes.Contains(data, []byte("X-Fortio-Multi-Id: 1")) {
			t.Errorf("Result %s doesn't contain expected X-Fortio-Multi-Id: 1", DebugSummary(data, 1024))
		}
		// Second request errors 100% so shouldn't be found
		if bytes.Contains(data, []byte("X-Fortio-Multi-Id: 2")) {
			t.Errorf("Result %s contains unexpected X-Fortio-Multi-Id: 2", DebugSummary(data, 1024))
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
