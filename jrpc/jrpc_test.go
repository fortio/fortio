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
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

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

// nolint: funlen,gocognit,maintidx // lots of tests using same server setup
func TestJPRC(t *testing.T) {
	prev := jrpc.SetCallTimeout(5 * time.Second)
	if prev != 60*time.Second {
		t.Errorf("Expected default call timeout to be 60 seconds, got %v", prev)
	}
	mux, addr := fhttp.HTTPServer("test", "0")
	port := addr.(*net.TCPAddr).Port
	var bad chan struct{}
	mux.HandleFunc("/test-api", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			err := jrpc.ReplyError(w, "should be a POST", nil)
			if err != nil {
				t.Errorf("Error in replying error: %v", err)
			}
			return
		}
		req, err := jrpc.HandleCall[Request](w, r)
		if err != nil {
			err = jrpc.ReplyError(w, "request error", err)
			if err != nil {
				t.Errorf("Error in replying error: %v", err)
			}
			return
		}
		if req.SomeInt == -7 {
			jrpc.ReplyNoPayload(w, 777)
			return
		}
		resp := Response{}
		if req.SomeInt == -8 {
			resp.Failed = true
			resp.Message = "simulated server error"
			jrpc.ReplyServerError(w, &resp)
			return
		}
		if req.SomeInt == -9 {
			// simulate a bad reply, invalid json
			w.WriteHeader(747)
			w.Write([]byte(`{bad}`))
			return
		}
		if req.SomeInt == -10 {
			// server error using unserializable struct
			err = jrpc.Reply(w, 200, &bad)
			if err == nil {
				t.Errorf("Expected bad serialization error")
			}
			return
		}
		resp.Message = "works"
		resp.InputInt = req.SomeInt
		// inefficient but this is just to test
		for _, s := range req.SomeString {
			resp.ConcatenatedStrings += s
		}
		jrpc.ReplyOk(w, &resp)
	})
	url := fmt.Sprintf("http://localhost:%d/test-api", port)
	req := Request{42, []string{"ab", "cd"}}
	res, err := jrpc.Call[Response](url, &req)
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
	// Error cases
	// Empty request, using Fetch()
	code, bytes, err := jrpc.Fetch(url)
	if err != nil {
		t.Errorf("failed Fetch: %v - %s", err, jrpc.DebugSummary(bytes, 256))
	}
	if code != http.StatusBadRequest {
		t.Errorf("expected status code 400, got %d - %s", code, jrpc.DebugSummary(bytes, 256))
	}
	res, err = jrpc.Deserialize[Response](bytes)
	if err != nil {
		t.Errorf("failed Deserialize: %v - %s", err, jrpc.DebugSummary(bytes, 256))
	}
	if !res.Failed {
		t.Errorf("response unexpectedly marked as not failed: %+v", res)
	}
	if res.Message != "should be a POST" {
		t.Errorf("response doesn't contain expected message: %+v", res)
	}
	// bad url
	badURL := "http://doesnotexist.fortio.org/"
	_, err = jrpc.Call[Response](badURL, &req)
	if err == nil {
		t.Errorf("expected error for bad url")
	}
	var de *net.DNSError
	if !errors.As(err, &de) {
		t.Errorf("expected dns error, got %v", err)
	}
	expected := "lookup doesnotexist.fortio.org"
	if de != nil && !strings.HasPrefix(de.Error(), expected) {
		t.Errorf("expected dns error to start with %q, got %q", expected, de.Error())
	}
	// bad json payload sent
	errReply, err := jrpc.CallWithPayload[jrpc.ErrorReply](url, []byte(`{foo: missing-quotes}`))
	if err == nil {
		t.Errorf("expected error, got nil and %v", res)
	}
	var fe *jrpc.FetchError
	if !errors.As(err, &fe) {
		t.Fatalf("error supposed to be FetchError %v: %v", res, err)
	}
	if fe.Code != http.StatusBadRequest {
		t.Errorf("expected status code %d, got %d", http.StatusBadRequest, fe.Code)
	}
	if errReply.Message != "request error" {
		t.Errorf("unexpected Message in %+v", errReply.Message)
	}
	expected = "invalid character 'f' looking for beginning of object key string"
	if errReply.Exception != expected {
		t.Errorf("expected Exception in body to be %q, got %+v", expected, errReply)
	}
	// bad json response, using Fetch()
	errReply, err = jrpc.CallNoPayload[jrpc.ErrorReply](url)
	if err == nil {
		t.Errorf("expected error %v", errReply)
	}
	if code != http.StatusBadRequest {
		t.Errorf("expected status code 400, got %d - %v - %v", code, err, errReply)
	}
	if !errReply.Failed {
		t.Errorf("response unexpectedly marked as not failed: %+v", res)
	}
	// trigger empty reply
	req.SomeInt = -7
	res, err = jrpc.Call[Response](url, &req)
	if err == nil {
		t.Errorf("error expected %v: %v", res, err)
	}
	if !errors.As(err, &fe) {
		t.Errorf("error supposed to be FetchError %v: %v", res, err)
	}
	if fe != nil && fe.Code != 777 {
		t.Errorf("error code expected for -7 to be 777: %v: %v", res, err)
	}
	// trigger server error
	req.SomeInt = -8
	res, err = jrpc.Call[Response](url, &req)
	if err == nil {
		t.Errorf("error expected %v: %v", res, err)
	}
	fe = nil
	if !errors.As(err, &fe) {
		t.Errorf("error supposed to be FetchError %v: %v", res, err)
	}
	if fe != nil && fe.Code != http.StatusServiceUnavailable {
		t.Errorf("error code expected for -8 to be 503: %v: %v", res, err)
	}
	if !res.Failed {
		t.Errorf("response supposed to be marked as failed: %+v", res)
	}
	if res.Message != "simulated server error" {
		t.Errorf("didn't get the error message expected for -8: %v: %v", res, err)
	}
	// trigger bad json response
	req.SomeInt = -9
	res, err = jrpc.Call[Response](url, &req)
	if err == nil {
		t.Errorf("error expected %v: %v", res, err)
	}
	fe = nil
	if !errors.As(err, &fe) {
		t.Errorf("error supposed to be FetchError %v: %v", res, err)
	}
	if fe != nil && fe.Code != 747 {
		t.Errorf("error code expected for -9 to be 747: %v: %v", res, err)
	}
	unwrap := fe.Unwrap()
	if unwrap == nil {
		t.Errorf("unwrapped error is nil: %+v", fe)
	}
	expected = "invalid character 'b' looking for beginning of object key string"
	if unwrap.Error() != expected {
		t.Errorf("unwrapped error expected to be %q, got %v", expected, unwrap.Error())
	}
	expected = "deserialization error, code 747: " + expected + " (raw reply: {bad})"
	if err.Error() != expected {
		t.Errorf("error string expected %q, got %q", expected, err.Error())
	}
	// trigger reply bad serialization
	req.SomeInt = -10
	res, err = jrpc.Call[Response](url, &req)
	if err == nil {
		t.Errorf("error expected %v", res)
	}
	expected = "deserialization error, code 500: unexpected end of JSON input (raw reply: )"
	if err.Error() != expected {
		t.Errorf("error string expected %q, got %q, %+v", expected, err.Error(), res)
	}
	// Unserializable client side
	res, err = jrpc.Call[Response](url, &bad)
	if err == nil {
		t.Errorf("error expected %v", res)
	}
	expected = "json: unsupported type: chan struct {}"
	if err.Error() != expected {
		t.Errorf("error string expected %q, got %q, %+v", expected, err.Error(), res)
	}
}

type ErrReader struct {
}

const ErrReaderMessage = "simulated IO error"

func (ErrReader) Read(p []byte) (n int, err error) {
	return 0, errors.New(ErrReaderMessage)
}

func TestHandleCallError(t *testing.T) {
	r, _ := http.NewRequest("GET", "/", ErrReader{})
	_, err := jrpc.HandleCall[jrpc.ReplyMessage](nil, r)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
	if err.Error() != ErrReaderMessage {
		t.Errorf("expected error %q, got %q", ErrReaderMessage, err.Error())
	}
}
