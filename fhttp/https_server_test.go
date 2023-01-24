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
	"testing"

	"fortio.org/fortio/log"
)

var (
	// Generated from "make cert".
	caCrt  = "../cert-tmp/ca.crt"
	svrCrt = "../cert-tmp/server.crt"
	svrKey = "../cert-tmp/server.key"
)

func TestHTTPSServer(t *testing.T) {
	log.SetLogLevel(log.Debug)
	m, a := ServeTLS("0", "/debug", svrCrt, svrKey)
	if m == nil || a == nil {
		t.Errorf("Failed to create server %v %v", m, a)
	}
	url := fmt.Sprintf("https://localhost:%d/debug", a.(*net.TCPAddr).Port)
	// Trigger transparent compression (which will add Accept-Encoding: gzip header)
	o := HTTPOptions{URL: url, DisableFastClient: true, TLSOptions: TLSOptions{CACert: caCrt}}
	client, _ := NewClient(&o)
	code, data, header := client.Fetch(context.Background())
	t.Logf("TestDebugHandlerSortedHeaders result code %d, data len %d, headerlen %d", code, len(data), header)
	if code != http.StatusOK {
		t.Errorf("Got %d instead of 200", code)
	}
	body := string(data)
	if !strings.Contains(body, "HTTP/") { // somehow with our go client it's not http2... (it is with plain curl)
		t.Errorf("Missing HTTP/ in body: %s", body)
	}
}
