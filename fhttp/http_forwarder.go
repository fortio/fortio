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

// Tee off traffic

package fhttp // import "fortio.org/fortio/fhttp"

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"sync"

	"fortio.org/fortio/fnet"
	"fortio.org/fortio/log"
)

var (
	// EnvoyRequestID is the header set by envoy and we need to propagate for distributed tracing.
	EnvoyRequestID = textproto.CanonicalMIMEHeaderKey("x-request-id")
	// TraceHeader is the single aggregated open tracing header to propagate when present.
	TraceHeader = textproto.CanonicalMIMEHeaderKey("b3")
	// TraceHeadersPrefix is the prefix for the multi header version of open zipkin.
	TraceHeadersPrefix = textproto.CanonicalMIMEHeaderKey("x-b3-")
)

// TargetConf is the structure to configure one of the multiple targets for MultiServer.
type TargetConf struct {
	Destination  string // Destination URL or base
	MirrorOrigin bool   // wether to use the incoming request as URI and data params to outgoing one (proxy like)
	//	Return       bool   // Will return the result of this target
}

// MultiServerConfig configures the MultiServer and holds the http client it uses for proxying.
type MultiServerConfig struct {
	Targets []TargetConf
	Serial  bool // Serialize or parallel queries
	//	Javascript bool // return data as UI suitable
	Name   string
	client *http.Client
}

func makeMirrorRequest(baseURL string, r *http.Request, data []byte) *http.Request {
	url := baseURL + r.RequestURI
	bodyReader := ioutil.NopCloser(bytes.NewReader(data))
	req, err := http.NewRequestWithContext(r.Context(), r.Method, url, bodyReader)
	if err != nil {
		log.Warnf("new mirror request error for %q: %v", url, err)
		return nil
	}
	// Copy all headers
	// Host header is not in Header so safe to copy
	CopyHeaders(req, r, true)
	return req
}

// CopyHeaders copies all or trace headers.
func CopyHeaders(req, r *http.Request, all bool) {
	// Copy only trace headers unless all is true.
	for k, v := range r.Header {
		if all || k == EnvoyRequestID || k == TraceHeader || strings.HasPrefix(k, TraceHeadersPrefix) {
			for _, vv := range v {
				req.Header.Add(k, vv)
			}
			log.Debugf("Adding header %q = %q", k, v)
		} else {
			log.Debugf("Skipping header %q", k)
		}
	}
}

// MakeSimpleRequest makes a new request for url but copies trace headers from input request r.
func MakeSimpleRequest(url string, r *http.Request) *http.Request {
	req, err := http.NewRequestWithContext(r.Context(), "GET", url, nil)
	if err != nil {
		log.Warnf("new request error for %q: %v", url, err)
		return nil
	}
	// Copy only trace headers
	CopyHeaders(req, r, false)
	req.Header.Add("User-Agent", userAgent)
	return req
}

// TeeHandler common part between TeeSerialHandler and TeeParallelHandler.
func (mcfg *MultiServerConfig) TeeHandler(w http.ResponseWriter, r *http.Request) {
	if log.LogVerbose() {
		LogRequest(r, mcfg.Name)
	}
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Errf("Error reading on %v: %v", r, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	r.Body.Close()
	if mcfg.Serial {
		mcfg.TeeSerialHandler(w, r, data)
	} else {
		mcfg.TeeParallelHandler(w, r, data)
	}
}

func setupRequest(r *http.Request, i int, t TargetConf, data []byte) *http.Request {
	var req *http.Request
	if t.MirrorOrigin {
		req = makeMirrorRequest(t.Destination, r, data)
	} else {
		req = MakeSimpleRequest(t.Destination, r)
	}
	if req == nil {
		// error already logged
		return nil
	}
	OnBehalfOfRequest(req, r)
	req.Header.Add("X-Fortio-Multi-ID", strconv.Itoa(i+1))
	log.LogVf("Going to %s", req.URL.String())
	return req
}

// TeeSerialHandler handles teeing off traffic in serial (one at a time) mode.
func (mcfg *MultiServerConfig) TeeSerialHandler(w http.ResponseWriter, r *http.Request, data []byte) {
	first := true
	for i, t := range mcfg.Targets {
		req := setupRequest(r, i, t, data)
		if req == nil {
			continue
		}
		url := req.URL.String()
		resp, err := mcfg.client.Do(req)
		if err != nil {
			msg := fmt.Sprintf("Error for %s: %v", url, err)
			log.Warnf(msg)
			if first {
				w.WriteHeader(http.StatusServiceUnavailable)
				first = false
			}
			_, _ = w.Write([]byte(msg))
			_, _ = w.Write([]byte("\n"))
			continue
		}
		if first {
			w.WriteHeader(resp.StatusCode)
			first = false
		}
		w, err := fnet.Copy(w, resp.Body)
		if err != nil {
			log.Warnf("Error copying response for %s: %v", url, err)
		}
		log.LogVf("copied %d from %s - code %d", w, url, resp.StatusCode)
		_ = resp.Body.Close()
	}
}

func singleRequest(client *http.Client, w io.Writer, req *http.Request, statusPtr *int) {
	url := req.URL.String()
	resp, err := client.Do(req)
	if err != nil {
		msg := fmt.Sprintf("Error for %s: %v", url, err)
		log.Warnf(msg)
		_, _ = w.Write([]byte(msg))
		_, _ = w.Write([]byte{'\n'})
		*statusPtr = -1
		return
	}
	*statusPtr = resp.StatusCode
	bw, err := fnet.Copy(w, resp.Body)
	if err != nil {
		log.Warnf("Error copying response for %s: %v", url, err)
	}
	log.LogVf("sr copied %d from %s - code %d", bw, url, resp.StatusCode)
	_ = resp.Body.Close()
}

// TeeParallelHandler handles teeing off traffic in parallel (one goroutine each) mode.
func (mcfg *MultiServerConfig) TeeParallelHandler(w http.ResponseWriter, r *http.Request, data []byte) {
	var wg sync.WaitGroup
	numTargets := len(mcfg.Targets)
	ba := make([]bytes.Buffer, numTargets)
	sa := make([]int, numTargets)
	for i := 0; i < numTargets; i++ {
		req := setupRequest(r, i, mcfg.Targets[i], data)
		if req == nil {
			continue
		}
		wg.Add(1)
		go func(client *http.Client, buffer *bytes.Buffer, request *http.Request, statusPtr *int) {
			writer := bufio.NewWriter(buffer)
			singleRequest(client, writer, request, statusPtr)
			writer.Flush()
			wg.Done()
		}(mcfg.client, &ba[i], req, &sa[i])
	}
	wg.Wait()
	// Get overall status only ok if all OK, first non ok sets status
	status := http.StatusOK
	for i := 0; i < numTargets; i++ {
		if sa[i] != http.StatusOK {
			status = sa[i]
			break
		}
	}
	if status <= 0 {
		status = http.StatusServiceUnavailable
	}
	w.WriteHeader(status)
	// Send all the data back to back
	for i := 0; i < numTargets; i++ {
		bw, err := w.Write(ba[i].Bytes())
		log.Debugf("For %d, wrote %d bytes - status %d", i, bw, sa[i])
		if err != nil {
			log.Warnf("Error writing back to %s: %v", r.RemoteAddr, err)
			break
		}
	}
}

// CreateProxyClient http client for connection reuse.
func CreateProxyClient() *http.Client {
	client := &http.Client{
		Transport: &http.Transport{
			// TODO make configurable, should be fine for now for most but extreme -c values
			MaxIdleConnsPerHost: 128, // must be more than incoming parallelization; divided by number of fan out if using parallel mode
			MaxIdleConns:        256,
		},
	}
	return client
}

// MultiServer starts fan out http server on the given port.
// Returns the mux and addr where the listening socket is bound.
// The port can be retrieved from it when requesting the 0 port as
// input for dynamic http server.
func MultiServer(port string, cfg *MultiServerConfig) (*http.ServeMux, net.Addr) {
	hName := cfg.Name
	if hName == "" {
		hName = "Multi on " + port // port could be :0 for dynamic...
	}
	mux, addr := HTTPServer(hName, port)
	if addr == nil {
		return nil, nil // error already logged
	}
	aStr := addr.String()
	if cfg.Name == "" {
		// get actual bound port in case of :0
		cfg.Name = "Multi on " + aStr
	}
	cfg.client = CreateProxyClient()
	for i := range cfg.Targets {
		t := &cfg.Targets[i]
		if t.MirrorOrigin {
			t.Destination = strings.TrimSuffix(t.Destination, "/") // remove trailing / because we will concatenate the request URI
		}
		if !strings.HasPrefix(t.Destination, "https://") && !strings.HasPrefix(t.Destination, "http://") {
			log.Infof("Assuming http:// on missing scheme for '%s'", t.Destination)
			t.Destination = "http://" + t.Destination
		}
	}
	log.Infof("Multi-server on %s running with %+v", aStr, cfg)
	mux.HandleFunc("/", cfg.TeeHandler)
	return mux, addr
}
