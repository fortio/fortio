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

package fhttp // import "fortio.org/fortio/fhttp"

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strconv"
	"strings"

	"fortio.org/fortio/fnet"
	"fortio.org/fortio/log"
) // Tee off traffic

// TargetConf is the structure to configure one of the multiple targets for MultiServer.
type TargetConf struct {
	Destination  string // Destination URL or base
	MirrorOrigin bool   // wether to use the incoming request as URI and data params to outgoing one (proxy like)
	//	Return       bool   // Will return the result of this target
}

// MultiServerConfig configures the MultiServer and holds the http client it uses for proxying.
type MultiServerConfig struct {
	Targets []TargetConf
	//	Serial     bool // Serialize or parallel queries
	//	Javascript bool // return data as UI suitable
	Name   string
	client *http.Client
}

// TeeHandler handles teeing off traffic.
func (mcfg *MultiServerConfig) TeeHandler(w http.ResponseWriter, r *http.Request) {
	LogRequest(r, mcfg.Name)
	first := true
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Errf("Error reading on %v: %v", r, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for i, t := range mcfg.Targets {
		url := t.Destination
		var req *http.Request
		var err error
		if t.MirrorOrigin {
			url = url + r.RequestURI
			bodyReader := ioutil.NopCloser(bytes.NewReader(data))
			req, err = http.NewRequest(r.Method, url, bodyReader)
			if err != nil {
				log.Warnf("new mirror request error for %q: %v", url, err)
				continue
			}
			// Copy all headers
			// Host header is not in Header so safe to copy
			for k, v := range r.Header {
				log.Debugf("Adding (all) headers - %q = %q", k, v)
				for _, vv := range v {
					req.Header.Add(k, vv)
				}
			}
		} else {
			req, err = http.NewRequest("GET", url, nil)
			if err != nil {
				log.Warnf("new request error for %q: %v", url, err)
				continue
			}
			// Copy only trace headers
			for k, v := range r.Header {
				if k == "x-request-id" || k == "b3" || strings.HasPrefix(k, "x-b3-") {
					for _, vv := range v {
						req.Header.Add(k, vv)
					}
					log.Debugf("Adding header %q = %q", k, v)
				} else {
					log.Debugf("Skipping header %q", k)
				}
			}
		}
		req.Header.Add("X-On-Behalf-Of", r.RemoteAddr)
		req.Header.Add("X-Fortio-Multi-ID", strconv.Itoa(i+1))
		log.LogVf("Going to %s", url)
		resp, err := mcfg.client.Do(req)
		if err != nil {
			msg := fmt.Sprintf("Error for %s: %v", url, err)
			log.Warnf(msg)
			if first {
				w.WriteHeader(http.StatusServiceUnavailable)
				first = false
			}
			w.Write([]byte(msg))
			w.Write([]byte("\n"))
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
	r.Body.Close()
}

// createClient http client for connection reuse
func createClient() *http.Client {
	client := &http.Client{
		/*
			Transport: &http.Transport{
				MaxIdleConnsPerHost: MaxIdleConnections,
			},
			Timeout: time.Duration(RequestTimeout) * time.Second,
		*/
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
	cfg.client = createClient()
	log.Infof("Multi-server on %s running with %+v", aStr, cfg)
	mux.HandleFunc("/", cfg.TeeHandler)
	return mux, addr
}
