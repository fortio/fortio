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

package jrpc_test

import (
	"fmt"
	"net"
	"net/http"
	"testing"

	"fortio.org/fortio/fhttp"
	"fortio.org/fortio/jrpc"
)

func TestDebugSummary(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"12345678", "12345678"},
		{"123456789", "123456789"},
		{"1234567890", "1234567890"},
		{"12345678901", "12345678901"},
		{"123456789012", "12: 1234...9012"},
		{"1234567890123", "13: 1234...0123"},
		{"12345678901234", "14: 1234...1234"},
		{"A\r\000\001\x80\nB", `A\r\x00\x01\x80\nB`},                   // escaping
		{"A\r\000Xyyyyyyyyy\001\x80\nB", `17: A\r\x00X...\x01\x80\nB`}, // escaping
	}
	for _, tst := range tests {
		if actual := jrpc.DebugSummary([]byte(tst.input), 8); actual != tst.expected {
			t.Errorf("Got '%s', expected '%s' for DebugSummary(%q)", actual, tst.expected, tst.input)
		}
	}
}

// Rest is also tested in rapi/restHandler_tests.go but that doesn't count for coverage

type Request struct {
	SomeInt    int
	SomeString []string
}

type Response struct {
	jrpc.ReplyMessage
	InputInt            int
	ConcatenatedStrings string
}

func TestJPRC(t *testing.T) {
	mux, addr := fhttp.HTTPServer("test", "0")
	port := addr.(*net.TCPAddr).Port
	mux.HandleFunc("/test-api", func(w http.ResponseWriter, r *http.Request) {
		req, err := jrpc.HandleCall[Request](w, r)
		if err != nil {
			t.Errorf("Got error %v", err)
		}
		resp := Response{}
		resp.Message = "works"
		resp.InputInt = req.SomeInt
		// inneficient but this is just to test
		for _, s := range req.SomeString {
			resp.ConcatenatedStrings += s
		}
		jrpc.ReplyOk(w, &resp)
	})
	url := fmt.Sprintf("http://localhost:%d/test-api", port)
	req := Request{42, []string{"ab", "cd"}}
	res, err := jrpc.Call[Request, Response](url, &req)
	if err != nil {
		t.Errorf("failed Call: %v", err)
	}
	if res.Failed {
		t.Errorf("response unexpectedly marked as failed: %+v", res)
	}
	if res.InputInt != 42 {
		t.Errorf("response doesn't contain expected int: %+v", res)
	}
	if res.ConcatenatedStrings != "abcd" {
		t.Errorf("response doesn't contain expected string: %+v", res)
	}
}
