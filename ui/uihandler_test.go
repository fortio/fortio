// Copyright 2017 Fortio Authors
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

package ui

import (
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"fortio.org/fortio/fhttp"
	"fortio.org/fortio/periodic"
)

func TestHandlerAllDoneWarmup(t *testing.T) {
	mux, addr := fhttp.DynamicHTTPServer(false)
	mux.HandleFunc("/echo/", fhttp.EchoHandler)
	var err error
	mainTemplate, err = template.ParseFS(templateFS, "templates/main.html", "templates/header.html")
	if err != nil {
		t.Fatalf("template: %v", err)
	}
	target := fmt.Sprintf("http://localhost:%d/echo/", addr.Port)
	tests := []struct {
		name   string
		params string
		want   string
	}{
		{"duration based run warms up each thread", "&t=100ms&qps=-1&c=3", "(plus 3 warmup)"},
		{"qps limited run still counts warmup before threads are lowered", "&t=1s&qps=2&c=10", "(plus 10 warmup)"},
		{"exact count run has no warmup", "&n=6&qps=-1&c=3", "(plus 0 warmup)"},
		{"default threads when c=0", "&t=100ms&qps=-1&c=0", fmt.Sprintf("(plus %d warmup)", periodic.DefaultRunnerOptions.NumThreads)},
		{"negative c means 1 thread", "&t=100ms&qps=-1&c=-2", "(plus 1 warmup)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/fortio/?load=Start&runner=http&url="+url.QueryEscape(target)+tt.params, nil)
			rec := httptest.NewRecorder()
			Handler(rec, req)
			body := rec.Body.String()
			if !strings.Contains(body, "All done") || !strings.Contains(body, tt.want) {
				t.Errorf("expected %q in All done line, got:\n%s", tt.want, body)
			}
		})
	}
}
