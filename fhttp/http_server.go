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

package fhttp // import "fortio.org/fortio/fhttp"

// pprof import to get /debug/pprof endpoints on a mux through SetupPPROF.

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/pprof"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"fortio.org/dflag"
	"fortio.org/fortio/fnet"
	"fortio.org/fortio/jrpc"
	"fortio.org/fortio/version"
	"fortio.org/log"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// -- Echo Server --

var (
	// Start time of the server (used in debug handler for uptime).
	startTime = time.Now()
	// EchoRequests is the number of request received. Only updated in Debug mode.
	EchoRequests            int64
	DefaultEchoServerParams = dflag.New("",
		"Default parameters/querystring to use if there isn't one provided explicitly. E.g \"status=404&delay=3s\"")
	Fetch2CopiesAllHeader = dflag.NewBool(true,
		"Determines if only tracing or all headers (and cookies) are copied from request on the fetch2 ui/server endpoint")
	ServerIdleTimeout = dflag.New(30*time.Second, "Default IdleTimeout for servers")
)

func Flush(w http.ResponseWriter) {
	f, ok := w.(http.Flusher)
	if ok {
		f.Flush()
	}
}

type FlushWriter struct {
	w http.ResponseWriter
}

func (fw FlushWriter) Write(p []byte) (n int, err error) {
	n, err = fw.w.Write(p)
	log.Debugf("FlushWriter wrote %d", n)
	Flush(fw.w)
	return
}

// QueryArg(r,...) is like r.FormValue(...) but exclusively
// getting the values from the query string (as we use the body for data).
// Result of parsing the query string is cached in r.Form so we don't keep
// parsing it and so r.Form can be used for multivalued entries like r.Form["header"].
func QueryArg(r *http.Request, key string) string {
	if r.Form == nil {
		r.Form = r.URL.Query()
	}
	return r.Form.Get(key)
}

// EchoHandler is an http server handler echoing back the input.
func EchoHandler(w http.ResponseWriter, r *http.Request) {
	// EchoHandler is an http server handler echoing back the input.
	if log.LogVerbose() {
		log.LogAndCall("Echo", func(w http.ResponseWriter, r *http.Request) {
			echoHandler(w, r)
		})(w, r)
		return
	}
	EchoHandler(w, r)
}

func echoHandler(w http.ResponseWriter, r *http.Request) {
	defaultParams := DefaultEchoServerParams.Get()
	hasQuestionMark := strings.Contains(r.RequestURI, "?")
	if !hasQuestionMark && len(defaultParams) > 0 {
		newQS := r.RequestURI + "?" + defaultParams
		log.LogVf("Adding default base query string %q to %v trying %q", defaultParams, r.URL, newQS)
		nr := *r
		var err error
		nr.URL, err = url.ParseRequestURI(newQS)
		if err != nil {
			log.Errf("Unexpected error parsing echo-server-default-params: %v", err)
		} else {
			nr.Form = nil
			r = &nr
		}
	}
	reqNum := handleCommonArgs(w, r)
	statusStr := QueryArg(r, "status")
	var status int
	if statusStr != "" {
		status = generateStatus(statusStr)
	} else {
		status = http.StatusOK
	}
	gzip := strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") && generateGzip(QueryArg(r, "gzip"))
	if gzip {
		gwz := NewGzipHTTPResponseWriter(w)
		defer gwz.Close()
		w = gwz
	}
	size := generateSize(QueryArg(r, "size")) // -1 means no size/payload mode
	var data []byte
	var err error
	// Also read the whole input if we're supposed to write something unrelated like size=100
	h2Mode := (r.ProtoMajor == 2) && (!gzip) && (size == -1)
	if !h2Mode {
		data, err = io.ReadAll(r.Body)
		log.Debugf("H1(.1) read %d", len(data))
		if err != nil {
			log.Errf("Error reading body: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if size >= 0 {
		log.LogVf("Writing %d size with %d status", size, status)
		writePayload(w, status, size)
		return
	}
	// echo back the Content-Type and Content-Length in the response
	for _, k := range []string{"Content-Type", "Content-Length"} {
		if v := r.Header.Get(k); v != "" {
			jrpc.SetHeaderIfMissing(w.Header(), k, v)
		}
	}
	if reqNum > 0 {
		jrpc.SetHeaderIfMissing(w.Header(), "x-fortio-id", strconv.FormatInt(reqNum, 10))
	}
	w.WriteHeader(status)
	if h2Mode {
		// h2 non gzip, non size case: stream the body back
		var n int64
		n, err = io.Copy(FlushWriter{w}, r.Body)
		log.Debugf("H2 read/Copied %d", n)
		if err != nil {
			log.Errf("Error copying from body to output: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		if _, err = w.Write(data); err != nil {
			log.Errf("Error writing response %v to %v", err, r.RemoteAddr)
		}
	}
}

// handleCommonArgs common flags for debug and echo handlers from query string only.
func handleCommonArgs(w http.ResponseWriter, r *http.Request) (rqNum int64) {
	dur := generateDelay(QueryArg(r, "delay"))
	if dur > 0 {
		log.LogVf("Sleeping for %v", dur)
		time.Sleep(dur)
	}
	if log.LogDebug() {
		// Note this easily lead to contention, debug mode only (or low qps).
		rqNum = atomic.AddInt64(&EchoRequests, 1)
		log.Debugf("Request # %v", rqNum)
	}
	if generateClose(QueryArg(r, "close")) {
		log.Debugf("Adding Connection:close / will close socket")
		w.Header().Set("Connection", "close")
	}
	// process header(s) args, must be before size to compose properly
	for _, hdr := range r.Form["header"] {
		log.LogVf("Adding requested header %s", hdr)
		if len(hdr) == 0 {
			continue
		}
		s := strings.SplitN(hdr, ":", 2)
		if len(s) != 2 {
			log.Errf("invalid extra header '%s', expecting Key: Value", hdr)
			continue
		}
		w.Header().Add(s[0], s[1])
	}
	return // rqNum ie 0 most of the time
}

func writePayload(w http.ResponseWriter, status int, size int) {
	jrpc.SetHeaderIfMissing(w.Header(), "Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(size))
	w.WriteHeader(status)
	n, err := w.Write(fnet.Payload[:size])
	if err != nil || n != size {
		log.Errf("Error writing payload of size %d: %d %v", size, n, err)
	}
}

func closingServer(listener net.Listener) error {
	var err error
	for {
		var c net.Conn
		c, err = listener.Accept()
		if err != nil {
			log.Errf("Accept error in dummy server %v", err)
			break
		}
		log.LogVf("Got connection from %v, closing", c.RemoteAddr())
		err = c.Close()
		if err != nil {
			log.Errf("Close error in dummy server %v", err)
			break
		}
	}
	return err
}

// HTTPServer creates an http server named name on address/port port.
// Port can include binding address and/or be port 0.
func HTTPServer(name string, port string) (*http.ServeMux, net.Addr) {
	m := http.NewServeMux()
	return m, HTTPServerWithHandler(name, port, m)
}

// HTTPServerWithHandler creates and h2c compatible server named name on address/port port.
// Port can include binding address and/or be port 0.
// Takes in a handler.
func HTTPServerWithHandler(name string, port string, hdlr http.Handler) net.Addr {
	h2s := &http2.Server{}
	s := &http.Server{
		ReadHeaderTimeout: ServerIdleTimeout.Get(),
		IdleTimeout:       ServerIdleTimeout.Get(),
		Handler:           h2c.NewHandler(hdlr, h2s),
		ErrorLog:          log.NewStdLogger("http2c srv "+name, log.Error),
	}
	listener, addr := fnet.Listen(name, port)
	if listener == nil {
		return nil // error already logged
	}
	go func() {
		err := s.Serve(listener)
		if err != nil {
			log.Fatalf("Unable to serve %s on %s: %v", name, addr.String(), err)
		}
	}()
	return addr
}

func HTTPSServer(name string, port string, to *TLSOptions) (*http.ServeMux, net.Addr) {
	listener, addr := fnet.Listen(name, port)
	if listener == nil {
		return nil, nil // error already logged
	}
	m := http.NewServeMux()
	tlsConfig, err := to.TLSConfig()
	if err != nil {
		return nil, nil
	}
	s := &http.Server{
		ReadHeaderTimeout: ServerIdleTimeout.Get(),
		IdleTimeout:       ServerIdleTimeout.Get(),
		Handler:           m,
		TLSConfig:         tlsConfig,
		ErrorLog:          log.NewStdLogger("http srv "+name, log.Error),
	}
	go func() {
		err := s.ServeTLS(listener, to.Cert, to.Key)
		if err != nil {
			log.Fatalf("Unable to TLS serve %s on %s: %v", name, addr.String(), err)
		}
	}()
	return m, addr
}

// DynamicHTTPServer listens on an available port, sets up an http or a closing
// server simulating an https server (when closing is true) server on it and
// returns the listening port and mux to which one can attach handlers to.
// Note: in a future version of istio, the closing will be actually be secure
// on/off and create an https server instead of a closing server.
// As this is a dynamic tcp socket server, the address is TCP.
func DynamicHTTPServer(closing bool) (*http.ServeMux, *net.TCPAddr) {
	if !closing {
		mux, addr := HTTPServer("dynamic", "0")
		return mux, addr.(*net.TCPAddr)
	}
	// Note: we actually use the fact it's not supported as an error server for tests - need to change that
	log.Warnf("Closing server requested (for error testing)")
	listener, addr := fnet.Listen("closing server", "0")
	go func() {
		err := closingServer(listener)
		if err != nil {
			log.Fatalf("Unable to serve closing server on %s: %v", addr.String(), err)
		}
	}()
	return nil, addr.(*net.TCPAddr)
}

/*
// DebugHandlerTemplate returns debug/useful info on the http requet.
// slower heavier but nicer source code version of DebugHandler
func DebugHandlerTemplate(w http.ResponseWriter, r *http.Request) {
	log.LogVf("%v %v %v %v", r.Method, r.URL, r.Proto, r.RemoteAddr)
	hostname, _ := os.Hostname()
	data, err := io.ReadAll(r.Body)
	if err != nil {
		log.Errf("Error reading %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Note: this looks nicer but is about 2x slower / less qps / more cpu and 25% bigger executable than doing the writes oneself:
	const templ = `Φορτίο version {{.Version}} echo debug server on {{.Hostname}} - request from {{.R.RemoteAddr}}

{{.R.Method}} {{.R.URL}} {{.R.Proto}}

headers:

{{ range $name, $vals := .R.Header }}{{range $val := $vals}}{{$name}}: {{ $val }}
{{end}}{{end}}
body:

{{.Body}}
{{if .DumpEnv}}
environment:
{{ range $idx, $e := .Env }}
{{$e}}{{end}}
{{end}}`
	t := template.Must(template.New("debugOutput").Parse(templ))
	err = t.Execute(w, &struct {
		R        *http.Request
		Hostname string
		Version  string
		Body     string
		DumpEnv  bool
		Env      []string
	}{r, hostname, Version, DebugSummary(data, 512), QueryArg(r,"env") == "dump", os.Environ()})
	if err != nil {
		Critf("Template execution failed: %v", err)
	}
	w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
}
*/

// DebugHandler returns debug/useful info to http client.
// Note this can be dangerous and shouldn't be exposed to the internet.
// A safer version is available as part of fortio's proxy
// https://github.com/fortio/proxy/blob/main/rp/reverse_proxy.go
func DebugHandler(w http.ResponseWriter, r *http.Request) {
	log.LogRequest(r, "Debug")
	var buf bytes.Buffer
	buf.WriteString("Φορτίο version ")
	buf.WriteString(version.Long())
	buf.WriteString(" echo debug server up for ")
	buf.WriteString(fmt.Sprint(RoundDuration(time.Since(startTime))))
	buf.WriteString(" on ")
	hostname, _ := os.Hostname()
	buf.WriteString(hostname)
	buf.WriteString(" - request from ")
	buf.WriteString(r.RemoteAddr)
	buf.WriteString(log.TLSInfo(r))
	buf.WriteString("\n\n")
	buf.WriteString(r.Method)
	buf.WriteByte(' ')
	buf.WriteString(r.URL.String())
	buf.WriteByte(' ')
	buf.WriteString(r.Proto)
	buf.WriteString("\n\nheaders:\n\n")
	// Host is removed from headers map and put here (!)
	buf.WriteString("Host: ")
	buf.WriteString(r.Host)

	keys := make([]string, 0, len(r.Header))
	for k := range r.Header {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, name := range keys {
		buf.WriteByte('\n')
		buf.WriteString(name)
		buf.WriteString(": ")
		first := true
		headers := r.Header[name]
		for _, h := range headers {
			if !first {
				buf.WriteByte(',')
			}
			buf.WriteString(h)
			first = false
		}
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		/*
			expected := r.ContentLength
			if expected < 0 {
				expected = 0 // GET have -1 content length
			}
			dataBuffer := make([]byte, expected)
			numRead, err := r.Body.Read(dataBuffer)
			log.LogVf("read %d/%d: %v", numRead, expected, err)
			data := dataBuffer[0:numRead]
			if err != nil && !errors.Is(err, io.EOF) {
		*/
		log.Errf("Error reading %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	buf.WriteString("\n\nbody:\n\n")
	buf.WriteString(DebugSummary(data, 512))
	buf.WriteByte('\n')
	if QueryArg(r, "env") == "dump" {
		buf.WriteString("\nenvironment:\n\n")
		for _, v := range os.Environ() {
			buf.WriteString(v)
			buf.WriteByte('\n')
		}
	}
	handleCommonArgs(w, r)
	jrpc.SetHeaderIfMissing(w.Header(), "Content-Type", "text/plain; charset=UTF-8")
	if _, err = w.Write(buf.Bytes()); err != nil {
		log.Errf("Error writing response %v to %v", err, r.RemoteAddr)
	}
	// Flush(w)
}

// CacheOn sets the header for indefinite caching.
func CacheOn(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "max-age=365000000, immutable")
}

// EchoDebugPath returns the additional echo handler path behind debugPath (ie /debug -> /debug/echo/).
func EchoDebugPath(debugPath string) string {
	return strings.TrimSuffix(debugPath, "/") + "/echo/"
}

// Serve starts a debug / echo http server on the given port.
// Returns the mux and addr where the listening socket is bound.
// The .Port can be retrieved from it when requesting the 0 port as
// input for dynamic http server.
func Serve(port, debugPath string) (*http.ServeMux, net.Addr) {
	return ServeTLS(port, debugPath, &TLSOptions{})
}

// ServeTLS starts a debug / echo server on the given port,
// using TLS if certPath and keyPath aren't not empty.
func ServeTLS(port, debugPath string, to *TLSOptions) (*http.ServeMux, net.Addr) {
	startTime = time.Now()
	var mux *http.ServeMux
	var addr net.Addr
	if to.Cert != "" && to.Key != "" {
		mux, addr = HTTPSServer("https-echo", port, to)
	} else {
		mux, addr = HTTPServer("http-echo", port)
	}
	if addr == nil {
		return nil, nil // error already logged
	}
	if debugPath != "" {
		mux.Handle(debugPath, Gzip(http.HandlerFunc(DebugHandler)))
		mux.HandleFunc(EchoDebugPath(debugPath), EchoHandler) // Fix #524
	}
	mux.HandleFunc("/", EchoHandler)
	return mux, addr
}

// ServeTCP is Serve() but restricted to TCP (return address is assumed
// to be TCP - will panic for unix domain).
func ServeTCP(port, debugPath string) (*http.ServeMux, *net.TCPAddr) {
	mux, addr := Serve(port, debugPath)
	if addr == nil {
		return nil, nil // error already logged
	}
	return mux, addr.(*net.TCPAddr)
}

// -- formerly in ui handler

// SetupPPROF add pprof to the mux (mirror the init() of http pprof).
func SetupPPROF(mux *http.ServeMux) {
	log.Warnf("pprof endpoints enabled on /debug/pprof/*")
	mux.HandleFunc("/debug/pprof/", log.LogAndCall("pprof:index", pprof.Index))
	mux.HandleFunc("/debug/pprof/cmdline", log.LogAndCall("pprof:cmdline", pprof.Cmdline))
	mux.HandleFunc("/debug/pprof/profile", log.LogAndCall("pprof:profile", pprof.Profile))
	mux.HandleFunc("/debug/pprof/symbol", log.LogAndCall("pprof:symbol", pprof.Symbol))
	mux.HandleFunc("/debug/pprof/trace", log.LogAndCall("pprof:trace", pprof.Trace))
}

// -- Fetch er (simple http proxy) --

var proxyClient = CreateProxyClient()

// FetcherHandler2 is the handler for the fetcher/proxy that supports h2 input and makes a
// new request with all headers copied (allows to test sticky routing)
// Note this should only be made available to trusted clients.
func FetcherHandler2(w http.ResponseWriter, r *http.Request) {
	log.LogRequest(r, "Fetch proxy2")
	query := r.URL.Query()
	vals, ok := query["url"]
	if !ok {
		http.Error(w, "missing url query argument", http.StatusBadRequest)
		return
	}
	url := strings.TrimSpace(vals[0])
	if url == "" {
		http.Error(w, "missing url value", http.StatusBadRequest)
		return
	}
	if !strings.HasPrefix(url, fnet.PrefixHTTP) && !strings.HasPrefix(url, fnet.PrefixHTTPS) {
		url = fnet.PrefixHTTP + url
	}
	req, opts := MakeSimpleRequest(url, r, Fetch2CopiesAllHeader.Get())
	if req == nil {
		http.Error(w, "parsing url failed, invalid url", http.StatusBadRequest)
		return
	}
	OnBehalfOfRequest(req, r)
	tr := proxyClient.Transport.(*http.Transport)
	if tr.TLSClientConfig == nil || tr.TLSClientConfig.InsecureSkipVerify != opts.Insecure {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: opts.Insecure} //nolint:gosec // as requested by the options.
	}
	resp, err := proxyClient.Do(req)
	if err != nil {
		msg := fmt.Sprintf("Error for %q: %v", url, err)
		log.Errf(msg)
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	log.LogVf("Success for %+v", req)
	for k, v := range resp.Header {
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}
	w.WriteHeader(resp.StatusCode)
	bw, err := fnet.Copy(w, resp.Body)
	if err != nil {
		log.Warnf("Error copying response for %s: %v", url, err)
	}
	log.LogVf("fh2 copied %d from %s - code %d", bw, url, resp.StatusCode)
	_ = resp.Body.Close()
}

// FetcherHandler is the handler for the fetcher/proxy.
func FetcherHandler(w http.ResponseWriter, r *http.Request) {
	log.LogRequest(r, "Fetch (prefix stripped)")
	hj, ok := w.(http.Hijacker)
	if !ok {
		log.Errf("hijacking not supported: %v", r.Proto)
		http.Error(w, "Use fetch2 when using http/2.0", http.StatusHTTPVersionNotSupported)
		return
	}
	conn, _, err := hj.Hijack()
	if err != nil {
		log.Errf("hijacking error %v", err)
		return
	}
	// Don't forget to close the connection:
	defer conn.Close()
	// Stripped prefix gets replaced by ./ - sometimes...
	url := strings.TrimPrefix(r.URL.String(), "./")
	opts := CommonHTTPOptionsFromForm(r)
	if opts.HTTPReqTimeOut == 0 {
		opts.HTTPReqTimeOut = 1 * time.Minute
	}
	opts.Init(url)
	OnBehalfOf(opts, r)
	//nolint:contextcheck // TODO: yes we should plug an aborter in the http options that's based on this request's context.
	client, _ := NewClient(opts)
	if client == nil {
		return // error logged already
	}
	_, data, _ := client.Fetch(r.Context())
	_, err = conn.Write(data)
	if err != nil {
		log.Errf("Error writing fetched data to %v: %v", r.RemoteAddr, err)
	}
	client.Close()
}

// -- Redirection to https feature --

// RedirectToHTTPSHandler handler sends a redirect to same URL with https.
func RedirectToHTTPSHandler(w http.ResponseWriter, r *http.Request) {
	dest := fnet.PrefixHTTPS + r.Host + r.URL.String()
	log.LogRequest(r, "Redirecting to "+dest)
	http.Redirect(w, r, dest, http.StatusSeeOther)
}

// RedirectToHTTPS Sets up a redirector to https on the given port.
// (Do not create a loop, make sure this is addressed from an ingress).
func RedirectToHTTPS(port string) net.Addr {
	return HTTPServerWithHandler("https redirector", port, http.HandlerFunc(RedirectToHTTPSHandler))
}

// Deprecated: use fortio.org/log.LogAndCall().
func LogAndCall(msg string, hf http.HandlerFunc) http.HandlerFunc {
	return log.LogAndCall(msg, hf)
}

// LogAndCallNoArg is LogAndCall for functions not needing the response/request args.
// Short cut for:
//
//	log.LogAndCall(msg, func(_ http.ResponseWriter, _ *http.Request) { f() })
func LogAndCallNoArg(msg string, f func()) http.HandlerFunc {
	return log.LogAndCall(msg, func(_ http.ResponseWriter, _ *http.Request) { f() })
}
