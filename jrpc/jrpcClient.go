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

// This package is a true self contained library, that doesn't rely on our logger nor other packages
// in fortio/ outside of version/ (which now also doesn't rely on logger or any other package).
// Naming is hard, we have Call, Send, Get, Fetch and FetchBytes pretty much all meaning retrieving data
// from a URL with the variants depending on whether we have something to serialize and if it's bytes
// or struct based in and out. Additionally *URL() variants are for when no additional headers or options
// are needed and the url is just a plain string. If golang supported multiple signatures it would be a single
// method name instead of 8.

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"time"

	"fortio.org/fortio/version"
	"fortio.org/sets"
)

// Client side and common code.

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

// Destination is the URL and optional additional headers.
// Depending on your needs consider also https://pkg.go.dev/fortio.org/multicurl/mc#MultiCurl
// and its configuration https://pkg.go.dev/fortio.org/multicurl/mc#Config object.
type Destination struct {
	URL string
	// Default is nil, which means no additional headers.
	Headers *http.Header
	// Default is 0 which means use global timeout.
	Timeout time.Duration
	// Default is "" which will use POST if there is a payload and GET otherwise.
	Method string
	// Context or will be context.Background() if not set.
	//nolint:containedctx // backward compatibility and keeping the many APIs simple as it's optional
	// https://go.dev/blog/context-and-structs
	Context context.Context
	// ClientTrace to use if set.
	ClientTrace *httptrace.ClientTrace
	// TLSConfig to use if set. This is ignored if HTTPClient is set.
	// Otherwise that setting this implies a new http.Client each call where this is set.
	TLSConfig *tls.Config
	// Ok codes. If nil (default) then 200, 201, 202 are ok.
	OkCodes sets.Set[int]
	// Only use this if all the options above are not enough. Defaults to http.DefaultClient.
	Client *http.Client
}

func (d *Destination) GetContext() context.Context {
	if d.Context != nil {
		return d.Context
	}
	return context.Background()
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
	return Fetch[Q](url, bytes)
}

// CallURL is Call without any options/non default headers, timeout etc and just the URL.
func CallURL[Q any, T any](url string, payload *T) (*Q, error) {
	return Call[Q](NewDestination(url), payload)
}

// Get fetches and deserializes the JSON returned by the Destination into a Q struct.
// Used when there is no json payload to send. Note that Get can be a different http
// method than GET, for instance if url.Method is set to "POST".
func Get[Q any](url *Destination) (*Q, error) {
	return Fetch[Q](url, []byte{})
}

// GetArray fetches and deserializes the JSON returned by the Destination into a slice of
// Q struct (ie the response is a json array).
func GetArray[Q any](url *Destination) ([]Q, error) {
	slicePtr, err := Fetch[[]Q](url, []byte{})
	if slicePtr == nil {
		return nil, err
	}
	return *slicePtr, err
}

// GetURL is Get without additional options (default timeout and headers).
func GetURL[Q any](url string) (*Q, error) {
	return Get[Q](NewDestination(url))
}

// Serialize serializes the object as json.
func Serialize(obj interface{}) ([]byte, error) {
	return json.Marshal(obj)
}

// Deserialize deserializes json as a new object of desired type.
func Deserialize[Q any](bytes []byte) (*Q, error) {
	var result Q
	if len(bytes) == 0 {
		// Allow empty body to be deserialized as empty object.
		return &result, nil
	}
	err := json.Unmarshal(bytes, &result)
	return &result, err // Will return zero object, not nil upon error
}

// Fetch is for cases where the payload is already serialized (or empty
// but call Get() when empty for clarity).
// Note that if you're looking for the []byte version instead of this
// generics version, it's now called FetchBytes().
func Fetch[Q any](url *Destination, bytes []byte) (*Q, error) {
	code, bytes, err := Send(url, bytes) // returns -1 on other errors
	if err != nil {
		return nil, err
	}
	var ok bool
	if url.OkCodes != nil {
		ok = url.OkCodes.Has(code)
	} else {
		// Default is 200, 201, 202 are ok
		ok = (code >= http.StatusOK && code <= http.StatusAccepted)
	}
	result, err := Deserialize[Q](bytes)
	if err != nil {
		if ok {
			return nil, err
		}
		return nil, &FetchError{"non ok http result and deserialization error", code, err, bytes}
	}
	if !ok {
		// can still be "ok" for some callers, they can use the result object as it deserialized as expected.
		return result, &FetchError{"non ok http result", code, nil, bytes}
	}
	return result, nil
}

// SetHeaderIfMissing utility function to not overwrite nor append to existing headers.
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
	ctx, cancel := context.WithTimeout(dest.GetContext(), curTimeout)
	defer cancel()
	var req *http.Request
	var err error
	var res []byte
	method := dest.Method
	if len(jsonPayload) > 0 {
		if method == "" {
			method = http.MethodPost
		}
		req, err = http.NewRequestWithContext(ctx, method, dest.URL, bytes.NewReader(jsonPayload))
	} else {
		if method == "" {
			method = http.MethodGet
		}
		req, err = http.NewRequestWithContext(ctx, method, dest.URL, nil)
	}
	if dest.ClientTrace != nil {
		req = req.WithContext(httptrace.WithClientTrace(req.Context(), dest.ClientTrace))
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
	var client *http.Client
	switch {
	case dest.Client != nil:
		client = dest.Client
	case dest.TLSConfig != nil:
		transport := http.DefaultTransport.(*http.Transport).Clone() // Let it crash/panic if somehow DefaultTransport is not a Transport
		transport.TLSClientConfig = dest.TLSConfig
		client = &http.Client{Transport: transport}
	default:
		client = http.DefaultClient
	}
	var resp *http.Response
	resp, err = client.Do(req)
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
// Technically this should be called FetchBytesURL().
func FetchURL(url string) (int, []byte, error) {
	return Send(NewDestination(url), []byte{})
}

// Fetch is Send without a payload (so will be a GET request).
// Used to be called Fetch() but we needed that shorter name to
// simplify the former CallWithPayload function name.
func FetchBytes(url *Destination) (int, []byte, error) {
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
