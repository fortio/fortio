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

// Opiniated JSON RPC / REST style library. Facilitates web JSON calls,
// using generics to serialize/deserialize any type.
package jrpc // import "fortio.org/fortio/jrpc"

// This package is a true self contained library, doesn't rely on our logger nor other packages in fortio/.
// Client side and common code.
import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"fortio.org/fortio/version"
)

const (
	UserAgentHeader = "User-Agent"
)

// Default timeout for Call.
var timeout = 60 * time.Second

// UserAgent is the User-Agent header used by client calls (also used in fhttp/).
var UserAgent = "fortio.org/fortio-" + version.Short()

// SetCallTimeout changes the timeout for further Call calls, returns
// the previous value (default in 60s). Value is used when a timeout
// isn't passed in the options. Note this is not thread safe,
// use Destination.Timeout for changing values outside of main/single
// thread.
func SetCallTimeout(t time.Duration) time.Duration {
	previous := timeout
	timeout = t
	return previous
}

// FetchError is a custom error type that preserves http result code if obtained.
type FetchError struct {
	Message string
	// HTTP code if present, -1 for other errors.
	Code int
	// Original (wrapped) error if any
	Err error
	// Original reply payload if any
	Bytes []byte
}

// Destination is the URL and optional additional headers
type Destination struct {
	URL     string
	Headers *http.Header
	Timeout time.Duration
}

func (fe *FetchError) Error() string {
	return fmt.Sprintf("%s, code %d: %v (raw reply: %s)", fe.Message, fe.Code, fe.Err, DebugSummary(fe.Bytes, 256))
}

func (fe *FetchError) Unwrap() error {
	return fe.Err
}

// Call calls the url endpoint, POSTing a serialized as json optional payload
// (pass nil for a GET http request) and returns the result, deserializing
// json into type Q. T can be inferred so we declare Response Q first.
func Call[Q any, T any](url *Destination, payload *T) (*Q, error) {
	var bytes []byte
	var err error
	if payload != nil {
		bytes, err = json.Marshal(payload)
		if err != nil {
			return nil, err
		}
	}
	return CallWithPayload[Q](url, bytes)
}

// CallURL is Call without any options/non default headers, timeout etc and just the URL.
func CallURL[Q any, T any](url string, payload *T) (*Q, error) {
	return Call[Q](NewDestination(url), payload)
}

// CallNoPayload is for an API call without json payload.
func CallNoPayload[Q any](url *Destination) (*Q, error) {
	return CallWithPayload[Q](url, []byte{})
}

func CallNoPayloadURL[Q any](url string) (*Q, error) {
	return CallWithPayload[Q](NewDestination(url), []byte{})
}

func Serialize(obj interface{}) ([]byte, error) {
	return json.Marshal(obj)
}

func Deserialize[Q any](bytes []byte) (*Q, error) {
	var result Q
	err := json.Unmarshal(bytes, &result)
	return &result, err // Will return zero object, not nil upon error
}

// CallWithPayload is for cases where the payload is already serialized (or empty).
func CallWithPayload[Q any](url *Destination, bytes []byte) (*Q, error) {
	code, bytes, err := Send(url, bytes) // returns -1 on other errors
	if err != nil {
		return nil, err
	}
	// 200, 201, 202 are ok
	ok := (code >= http.StatusOK && code <= http.StatusAccepted)
	result, err := Deserialize[Q](bytes)
	if err != nil {
		if ok {
			return nil, err
		}
		return nil, &FetchError{"deserialization error", code, err, bytes}
	}
	if !ok {
		// can still be "ok" for some callers, they can use the result object as it deserialized as expected.
		return result, &FetchError{"non ok http result", code, nil, bytes}
	}
	return result, nil
}

func SetHeaderIfMissing(headers http.Header, name, value string) {
	if headers.Get(name) != "" {
		return
	}
	headers.Set(name, value)
}

// Send fetches the result from url and sends optional payload as a POST, GET if missing.
// Returns the http code (if no other error before then, -1 if there are errors),
// the bytes from the reply and error if any.
func Send(dest *Destination, jsonPayload []byte) (int, []byte, error) {
	curTimeout := dest.Timeout
	if curTimeout == 0 {
		curTimeout = timeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), curTimeout)
	defer cancel()
	var req *http.Request
	var err error
	var res []byte
	if len(jsonPayload) > 0 {
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, dest.URL, bytes.NewReader(jsonPayload))
	} else {
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, dest.URL, nil)
	}
	if err != nil {
		return -1, res, err
	}
	if dest.Headers != nil {
		req.Header = dest.Headers.Clone()
	}
	if len(jsonPayload) > 0 {
		SetHeaderIfMissing(req.Header, "Content-Type", "application/json; charset=utf-8")
	}
	SetHeaderIfMissing(req.Header, "Accept", "application/json")
	SetHeaderIfMissing(req.Header, UserAgentHeader, UserAgent)
	var resp *http.Response
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		return -1, res, err
	}
	res, err = io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp.StatusCode, res, err
}

// NewDestination returns a Destination object set for the given url
// (and default/nil replacement headers and default global timeout).
func NewDestination(url string) *Destination {
	return &Destination{URL: url}
}

// FetchURL is Send without a payload and no additional options (default timeout and headers).
func FetchURL(url string) (int, []byte, error) {
	return Send(NewDestination(url), []byte{})
}

func Fetch(url *Destination) (int, []byte, error) {
	return Send(url, []byte{})
}

// EscapeBytes returns printable string. Same as %q format without the
// surrounding/extra "".
func EscapeBytes(buf []byte) string {
	e := fmt.Sprintf("%q", buf)
	return e[1 : len(e)-1]
}

// DebugSummary returns a string with the size and escaped first max/2 and
// last max/2 bytes of a buffer (or the whole escaped buffer if small enough).
func DebugSummary(buf []byte, max int) string {
	l := len(buf)
	if l <= max+3 { // no point in shortening to add ... if we could return those 3
		return EscapeBytes(buf)
	}
	max /= 2
	return fmt.Sprintf("%d: %s...%s", l, EscapeBytes(buf[:max]), EscapeBytes(buf[l-max:]))
}
