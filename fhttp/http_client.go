// Copyright 2017 Istio Authors
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
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"fortio.org/fortio/stats"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"fortio.org/fortio/fnet"
	"fortio.org/fortio/log"
	"fortio.org/fortio/version"
	"github.com/google/uuid"
)

// Fetcher is the Url content fetcher that the different client implements.
type Fetcher interface {
	// Fetch returns http code, data, offset of body (for client which returns
	// headers)
	Fetch() (int, []byte, int)
	// Close() cleans up connections and state - must be paired with NewClient calls.
	// returns how many sockets have been used (Fastclient only)
	Close() int
	// GetIPAddress() returns the last ip address used by this client connection.
	GetIPAddress() string
}

const (
	uuidToken = "{uuid}"
)

var (
	// BufferSizeKb size of the buffer (max data) for optimized client in kilobytes defaults to 128k.
	BufferSizeKb = 128
	// CheckConnectionClosedHeader indicates whether to check for server side connection closed headers.
	CheckConnectionClosedHeader = false
	// 'constants', case doesn't matter for those 3.
	contentLengthHeader   = []byte("\r\ncontent-length:")
	connectionCloseHeader = []byte("\r\nconnection: close")
	chunkedHeader         = []byte("\r\nTransfer-Encoding: chunked")
	rander                = NewSyncReader(rand.New(rand.NewSource(time.Now().UnixNano()))) // nolint: gosec // we want fast not crypto
)

// NewHTTPOptions creates and initialize a HTTPOptions object.
// It replaces plain % to %25 in the url. If you already have properly
// escaped URLs use o.URL = to set it.
func NewHTTPOptions(url string) *HTTPOptions {
	h := HTTPOptions{}
	return h.Init(url)
}

// Init initializes the headers in an HTTPOptions (User-Agent).
func (h *HTTPOptions) Init(url string) *HTTPOptions {
	if h.initDone {
		return h
	}
	h.initDone = true
	h.URL = url
	h.NumConnections = 1
	if h.HTTPReqTimeOut == 0 {
		log.Debugf("Request timeout not set, using default %v", HTTPReqTimeOutDefaultValue)
		h.HTTPReqTimeOut = HTTPReqTimeOutDefaultValue
	}
	if h.HTTPReqTimeOut < 0 {
		log.Warnf("Invalid timeout %v, setting to %v", h.HTTPReqTimeOut, HTTPReqTimeOutDefaultValue)
		h.HTTPReqTimeOut = HTTPReqTimeOutDefaultValue
	}
	h.URLSchemeCheck()
	return h
}

const (
	contentType   = "Content-Type"
	contentLength = "Content-Length"
)

// GenerateHeaders completes the header generation, including Content-Type/Length
// and user credential coming from the http options in addition to extra headers
// coming from flags and AddAndValidateExtraHeader().
// Warning this gets called more than once, do not generate duplicate headers.
func (h *HTTPOptions) GenerateHeaders() http.Header {
	if h.extraHeaders == nil { // not already initialized from flags.
		h.InitHeaders()
	}
	allHeaders := h.extraHeaders
	payloadLen := len(h.Payload)
	// If content-type isn't already specified and we have a payload, let's use the
	// standard for binary content:
	if payloadLen > 0 && len(h.ContentType) == 0 && len(allHeaders.Get(contentType)) == 0 {
		h.ContentType = "application/octet-stream"
	}
	if len(h.ContentType) > 0 {
		allHeaders.Set(contentType, h.ContentType)
	}
	// Add content-length unless already set in custom headers (or we're not doing a POST)
	if (payloadLen > 0 || len(h.ContentType) > 0) && len(allHeaders.Get(contentLength)) == 0 {
		allHeaders.Set(contentLength, strconv.Itoa(payloadLen))
	}
	err := h.ValidateAndAddBasicAuthentication(allHeaders)
	if err != nil {
		log.Errf("User credential is not valid: %v", err)
	}
	return allHeaders
}

// URLSchemeCheck makes sure the client will work with the scheme requested.
// it also adds missing http:// to emulate curl's behavior.
func (h *HTTPOptions) URLSchemeCheck() {
	log.LogVf("URLSchemeCheck %+v", h)
	if len(h.URL) == 0 {
		log.Errf("Unexpected init with empty url")
		return
	}
	hs := "https://" // longer of the 2 prefixes
	lcURL := h.URL
	if len(lcURL) > len(hs) {
		lcURL = strings.ToLower(h.URL[:len(hs)]) // no need to tolower more than we check
	}
	if strings.HasPrefix(lcURL, hs) {
		h.https = true
		return // url is good
	}
	if !strings.HasPrefix(lcURL, "http://") {
		log.Warnf("Assuming http:// on missing scheme for '%s'", h.URL)
		h.URL = "http://" + h.URL
	}
}

var userAgent = "fortio.org/fortio-" + version.Short()

const (
	retcodeOffset = len("HTTP/1.X ")
	// HTTPReqTimeOutDefaultValue is the default timeout value. 3s.
	HTTPReqTimeOutDefaultValue = 3 * time.Second
)

// HTTPOptions holds the common options of both http clients and the headers.
type HTTPOptions struct {
	TLSOptions
	URL               string
	NumConnections    int  // num connections (for std client)
	Compression       bool // defaults to no compression, only used by std client
	DisableFastClient bool // defaults to fast client
	HTTP10            bool // defaults to http1.1
	DisableKeepAlive  bool // so default is keep alive
	AllowHalfClose    bool // if not keepalive, whether to half close after request
	FollowRedirects   bool // For the Std Client only: follow redirects.
	initDone          bool
	https             bool   // whether URLSchemeCheck determined this was an https:// call or not
	Resolve           string // resolve Common Name to this ip when use CN as target url
	// ExtraHeaders to be added to each request (UserAgent and headers set through AddAndValidateExtraHeader()).
	extraHeaders http.Header
	// Host is treated specially, remember that virtual header separately.
	hostOverride     string
	HTTPReqTimeOut   time.Duration // timeout value for http request
	UserCredentials  string        // user credentials for authorization
	ContentType      string        // indicates request body type, implies POST instead of GET
	Payload          []byte        // body for http request, implies POST if not empty.
	LogErrors        bool          // whether to log non 2xx code as they occur or not
	ID               int           // id to use for logging (thread id when used as a runner)
	SequentialWarmup bool          // whether to do http(s) runs warmup sequentially or in parallel (new default is //)
	ConnReuseRange   [2]int        // range of max number of connection to reuse for each thread.
	// re-resolve the DNS name when the connection breaks.
	NoResolveEachConn bool
}

// ResetHeaders resets all the headers, including the User-Agent: one (and the Host: logical special header).
// This is used from the UI as the user agent is settable from the form UI.
func (h *HTTPOptions) ResetHeaders() {
	h.extraHeaders = make(http.Header)
	h.hostOverride = ""
}

// InitHeaders initialize and/or resets the default headers (ie just User-Agent).
func (h *HTTPOptions) InitHeaders() {
	h.ResetHeaders()
	h.extraHeaders.Add("User-Agent", userAgent)
	// No other headers should be added here based on options content as this is called only once
	// before command line option -H are parsed/set.
}

// PayloadUTF8 returns the payload as a string. If payload is null return empty string
// This is only needed due to grpc ping proto. It takes string instead of byte array.
func (h *HTTPOptions) PayloadUTF8() string {
	p := h.Payload
	pl := len(p)
	if pl == 0 {
		return ""
	}
	// grpc doesn't like invalid utf-8 strings, get rid of them
	res := strings.ToValidUTF8(string(p), "")
	l := len([]byte(res))
	if l < pl {
		// but then keep the expected bytes length (though it'll compressed unlike the original)
		pad := pl - l
		res += strings.Repeat("X", pad)
		log.Infof("Padded payload with %d extra Xs to make valid UTF-8 after filtering invalid sequences", pad)
		log.Debugf("Payload now %d bytes", len(res))
	}
	return res
}

// ValidateAndAddBasicAuthentication validates user credentials and adds basic authentication to http header,
// if user credentials are valid.
func (h *HTTPOptions) ValidateAndAddBasicAuthentication(headers http.Header) error {
	if len(h.UserCredentials) == 0 {
		return nil // user credential is not entered
	}
	s := strings.SplitN(h.UserCredentials, ":", 2)
	if len(s) != 2 {
		return fmt.Errorf("invalid user credentials \"%s\", expecting \"user:password\"", h.UserCredentials)
	}
	headers.Set("Authorization", generateBase64UserCredentials(h.UserCredentials))
	return nil
}

// AllHeaders returns the current set of headers including virtual/special Host header.
func (h *HTTPOptions) AllHeaders() http.Header {
	headers := h.GenerateHeaders()
	if h.hostOverride != "" {
		headers.Add("Host", h.hostOverride)
	}
	return headers
}

// Method returns the method of the http req.
func (h *HTTPOptions) Method() string {
	if len(h.Payload) > 0 || h.ContentType != "" {
		return fnet.POST
	}
	return fnet.GET
}

// AddAndValidateExtraHeader collects extra headers (see commonflags.go for example).
func (h *HTTPOptions) AddAndValidateExtraHeader(hdr string) error {
	// This function can be called from the flag settings, before we have a URL
	// so we can't just call h.Init(h.URL)
	if h.extraHeaders == nil {
		h.InitHeaders()
	}
	s := strings.SplitN(hdr, ":", 2)
	if len(s) != 2 {
		return fmt.Errorf("invalid extra header '%s', expecting Key: Value", hdr)
	}
	key := strings.TrimSpace(s[0])
	value := strings.TrimSpace(s[1])
	if strings.EqualFold(key, "host") {
		log.LogVf("Will be setting special Host header to %s", value)
		h.hostOverride = value
	} else {
		log.LogVf("Setting regular extra header %s: %s", key, value)
		h.extraHeaders.Add(key, value)
		log.Debugf("headers now %+v", h.extraHeaders)
	}
	return nil
}

func (h *HTTPOptions) ValidateAndSetConnectionReuseRange(inp string) error {
	if inp == "" {
		return nil
	}

	reuseRangeString := strings.Split(inp, ":")
	if len(reuseRangeString) > 2 {
		return fmt.Errorf("more than two integers were provided in the connection reuse range")
	}
	reuseRangeInt := make([]int, 2)
	for i, input := range reuseRangeString {
		val, err := strconv.Atoi(input)
		if err != nil {
			return fmt.Errorf("invalid value for connection reuse range, err: %w", err)
		}
		reuseRangeInt[i] = val
	}

	if len(reuseRangeString) == 1 {
		h.ConnReuseRange = [2]int{reuseRangeInt[0], reuseRangeInt[0]}
	} else {
		if reuseRangeInt[0] < reuseRangeInt[1] {
			h.ConnReuseRange = [2]int{reuseRangeInt[0], reuseRangeInt[1]}
		} else {
			h.ConnReuseRange = [2]int{reuseRangeInt[1], reuseRangeInt[0]}
		}
	}

	return nil
}

// newHttpRequest makes a new http GET request for url with User-Agent.
func newHTTPRequest(o *HTTPOptions) (*http.Request, error) {
	method := o.Method()
	var body io.Reader
	if method == fnet.POST {
		body = bytes.NewReader(o.Payload)
	}
	req, err := http.NewRequest(method, o.URL, body)
	if err != nil {
		log.Errf("[%d] Unable to make %s request for %s : %v", o.ID, method, o.URL, err)
		return nil, err
	}
	req.Header = o.GenerateHeaders()
	if o.hostOverride != "" {
		req.Host = o.hostOverride
	}
	if !log.LogDebug() {
		return req, nil
	}
	bytes, err := httputil.DumpRequestOut(req, false)
	if err != nil {
		log.Errf("[%d] Unable to dump request: %v", o.ID, err)
	} else {
		log.Debugf("[%d] For URL %s, sending:\n%s", o.ID, o.URL, bytes)
	}
	return req, nil
}

// Client object for making repeated requests of the same URL using the same
// http client (net/http).
// TODO: refactor common parts with FastClient.
type Client struct {
	url                  string
	path                 string // original path of the request's url
	rawQuery             string // original query params
	body                 []byte // original body of the request
	req                  *http.Request
	client               *http.Client
	transport            *http.Transport
	pathContainsUUID     bool // if url contains the "{uuid}" pattern (lowercase)
	rawQueryContainsUUID bool // if any query params contains the "{uuid}" pattern (lowercase)
	bodyContainsUUID     bool // if body contains the "{uuid}" pattern (lowercase)
	logErrors            bool
	id                   int
	socketCount          int
}

// Close cleans up any resources used by NewStdClient.
func (c *Client) Close() int {
	log.Debugf("[%d] Close() on %+v", c.id, c)
	if c.req != nil {
		if c.req.Body != nil {
			if err := c.req.Body.Close(); err != nil {
				log.Warnf("[%d] Error closing std client body: %v", c.id, err)
			}
		}
		c.req = nil
	}
	if c.transport != nil {
		c.transport.CloseIdleConnections()
	}
	return c.socketCount
}

// ChangeURL only for standard client, allows fetching a different URL.
func (c *Client) ChangeURL(urlStr string) (err error) {
	c.url = urlStr
	c.req.URL, err = url.Parse(urlStr)
	return err
}

// Fetch fetches the byte and code for pre created client.
func (c *Client) Fetch() (int, []byte, int) {
	// req can't be null (client itself would be null in that case)
	if c.pathContainsUUID {
		path := c.path
		for strings.Contains(path, uuidToken) {
			path = strings.Replace(path, uuidToken, generateUUID(), 1)
		}
		c.req.URL.Path = path
	}
	if c.rawQueryContainsUUID {
		rawQuery := c.rawQuery
		for strings.Contains(rawQuery, uuidToken) {
			rawQuery = strings.Replace(rawQuery, uuidToken, generateUUID(), 1)
		}

		c.req.URL.RawQuery = rawQuery
	}
	if c.bodyContainsUUID {
		body := string(c.body)
		for strings.Contains(body, uuidToken) {
			body = strings.Replace(body, uuidToken, generateUUID(), 1)
		}
		bodyBytes := []byte(body)
		c.req.ContentLength = int64(len(bodyBytes))
		c.req.Body = ioutil.NopCloser(bytes.NewReader(bodyBytes))
	} else if len(c.body) > 0 {
		c.req.Body = ioutil.NopCloser(bytes.NewReader(c.body))
	}

	resp, err := c.client.Do(c.req)
	if err != nil {
		log.Errf("[%d] Unable to send %s request for %s : %v", c.id, c.req.Method, c.url, err)
		return -1, []byte(err.Error()), 0
	}
	var data []byte
	if log.LogDebug() {
		if data, err = httputil.DumpResponse(resp, false); err != nil {
			log.Errf("[%d] Unable to dump response %v", c.id, err)
		} else {
			log.Debugf("[%d] For URL %s, received:\n%s", c.id, c.url, data)
		}
	}
	data, err = ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		log.Errf("[%d] Unable to read response for %s : %v", c.id, c.url, err)
		code := resp.StatusCode
		if codeIsOK(code) {
			code = http.StatusNoContent
			log.Warnf("[%d] Ok code despite read error, switching code to %d", c.id, code)
		}
		return code, data, 0
	}
	code := resp.StatusCode
	log.Debugf("[%d] Got %d : %s for %s %s - response is %d bytes", c.id, code, resp.Status, c.req.Method, c.url, len(data))
	if c.logErrors && !codeIsOK(code) {
		log.Warnf("[%d] Non ok http code %d", c.id, code)
	}
	return code, data, 0
}

// GetIPAddress get the ip address that DNS resolves to when using stdClient.
func (c *Client) GetIPAddress() string {
	return c.req.RemoteAddr
}

// NewClient creates either a standard or fast client (depending on
// the DisableFastClient flag).
func NewClient(o *HTTPOptions) (Fetcher, error) {
	o.Init(o.URL) // For completely new options
	// For changes to options after init
	o.URLSchemeCheck()
	if o.DisableFastClient {
		return NewStdClient(o)
	}
	return NewFastClient(o)
}

// NewStdClient creates a client object that wraps the net/http standard client.
func NewStdClient(o *HTTPOptions) (*Client, error) {
	o.Init(o.URL) // also normalizes NumConnections etc to be valid.
	req, err := newHTTPRequest(o)
	if req == nil {
		return nil, err
	}

	client := Client{
		url:                  o.URL,
		path:                 req.URL.Path,
		pathContainsUUID:     strings.Contains(req.URL.Path, uuidToken),
		rawQuery:             req.URL.RawQuery,
		rawQueryContainsUUID: strings.Contains(req.URL.RawQuery, uuidToken),
		body:                 o.Payload,
		bodyContainsUUID:     strings.Contains(string(o.Payload), uuidToken),
		req:                  req,
		client: &http.Client{
			Timeout: o.HTTPReqTimeOut,
		},
		id:        o.ID,
		logErrors: o.LogErrors,
	}

	tr := http.Transport{
		MaxIdleConns:        o.NumConnections,
		MaxIdleConnsPerHost: o.NumConnections,
		DisableCompression:  !o.Compression,
		DisableKeepAlives:   o.DisableKeepAlive,
		Proxy:               http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			// redirect all connections to resolved ip, and use cn as sni host
			if o.Resolve != "" {
				addr = o.Resolve + addr[strings.LastIndex(addr, ":"):]
			}
			var conn net.Conn
			conn, err = (&net.Dialer{
				Timeout: o.HTTPReqTimeOut,
			}).DialContext(ctx, network, addr)

			if conn != nil {
				newRemoteAddress := conn.RemoteAddr().String()
				// No change when it wasn't set before (first time) and when the value isn't actually changing either.
				if req.RemoteAddr != "" && newRemoteAddress != req.RemoteAddr {
					log.Infof("[%d] Standard client IP address changed from %s to %s", client.id, req.RemoteAddr, newRemoteAddress)
				}
				req.RemoteAddr = newRemoteAddress
				client.socketCount++
			}

			return conn, err
		},
		TLSHandshakeTimeout: o.HTTPReqTimeOut,
	}

	client.client.Transport = &tr
	client.transport = &tr

	if o.https {
		tr.TLSClientConfig, err = o.TLSOptions.TLSClientConfig()
		if err != nil {
			return nil, err
		}
	}
	if !o.FollowRedirects {
		// Lets us see the raw response instead of auto following redirects.
		client.client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	return &client, nil
}

// FetchURL fetches the data at the given url using the standard client and default options.
// Returns the http status code (http.StatusOK == 200 for success) and the data.
// To be used only for single fetches or when performance doesn't matter as the client is closed at the end.
func FetchURL(url string) (int, []byte) {
	o := NewHTTPOptions(url)
	// Maximize chances of getting the data back, vs the raw payload like the fast client
	o.DisableFastClient = true
	o.FollowRedirects = true
	return Fetch(o)
}

// Fetch creates a client an performs a fetch according to the http options passed in.
// To be used only for single fetches or when performance doesn't matter as the client is closed at the end.
func Fetch(httpOptions *HTTPOptions) (int, []byte) {
	cli, _ := NewClient(httpOptions)
	code, data, _ := cli.Fetch()
	cli.Close()
	return code, data
}

// FastClient is a fast, lockfree single purpose http 1.0/1.1 client.
type FastClient struct {
	buffer       []byte
	req          []byte
	dest         net.Addr
	socket       net.Conn
	socketCount  int
	size         int
	code         int
	errorCount   int
	headerLen    int
	url          string
	host         string
	hostname     string
	port         string
	http10       bool // http 1.0, simplest: no Host, forced no keepAlive, no parsing
	keepAlive    bool
	parseHeaders bool // don't bother in http/1.0
	halfClose    bool // allow/do half close when keepAlive is false
	reqTimeout   time.Duration
	uuidMarkers  [][]byte
	logErrors    bool
	id           int
	https        bool
	tlsConfig    *tls.Config
	// Resolve the DNS name for each connection
	resolve           string
	noResolveEachConn bool
	ipAddrUsage       *stats.Occurrence
	// range of connection reuse threshold that current thread will choose from
	connReuseRange [2]int
	connReuse      int
	reuseCount     int
}

// GetIPAddress get ip address that DNS resolved to when using fast client.
func (c *FastClient) GetIPAddress() string {
	return c.dest.String()
}

// Close cleans up any resources used by FastClient.
func (c *FastClient) Close() int {
	log.Debugf("[%d] Closing %p %s socket count %d", c.id, c, c.url, c.socketCount)
	if c.socket != nil {
		if err := c.socket.Close(); err != nil {
			log.Warnf("[%d] Error closing fast client's socket: %v", c.id, err)
		}
		c.socket = nil
	}
	return c.socketCount
}

// NewFastClient makes a basic, efficient http 1.0/1.1 client.
// This function itself doesn't need to be super efficient as it is created at
// the beginning and then reused many times.
func NewFastClient(o *HTTPOptions) (Fetcher, error) { // nolint: funlen
	method := o.Method()
	payloadLen := len(o.Payload)
	o.Init(o.URL)
	proto := "1.1"
	if o.HTTP10 {
		proto = "1.0"
	}

	uuidStrings := []string{}
	urlString := o.URL
	for strings.Contains(urlString, uuidToken) {
		uuidString := generateUUID()
		uuidStrings = append(uuidStrings, uuidString)
		urlString = strings.Replace(urlString, uuidToken, uuidString, 1)
	}
	payload := string(o.Payload)
	for strings.Contains(payload, uuidToken) {
		uuidString := generateUUID()
		uuidStrings = append(uuidStrings, uuidString)
		payload = strings.Replace(payload, uuidToken, uuidString, 1)
	}
	if len(uuidStrings) > 0 {
		o.Payload = []byte(payload)
	}
	// Parse the url, extract components.
	url, err := url.Parse(urlString)
	if err != nil {
		log.Errf("[%d] Bad url %q : %v", o.ID, urlString, err)
		return nil, err
	}

	if o.NoResolveEachConn && o.Resolve != "" {
		log.Warnf("Both `-resolve` and `-no-reresolve` flags are defined, will use same ip address on new conn")
	}

	// Randomly assign a max connection reuse threshold to this thread.
	var connReuse int
	if o.ConnReuseRange != [2]int{0, 0} {
		connReuse = generateReuseThreshold(o.ConnReuseRange[0], o.ConnReuseRange[1])
	}

	// note: Host includes the port
	bc := FastClient{
		url: o.URL, host: url.Host, hostname: url.Hostname(), port: url.Port(),
		http10: o.HTTP10, halfClose: o.AllowHalfClose, logErrors: o.LogErrors, id: o.ID,
		https: o.https, connReuseRange: o.ConnReuseRange, connReuse: connReuse,
		resolve: o.Resolve, noResolveEachConn: o.NoResolveEachConn, ipAddrUsage: stats.NewOccurrence(),
	}
	if o.https {
		bc.tlsConfig, err = o.TLSOptions.TLSClientConfig()
		if err != nil {
			return nil, err
		}
	}
	bc.buffer = make([]byte, BufferSizeKb*1024)
	if bc.port == "" {
		bc.port = url.Scheme // ie http which turns into 80 later
		log.LogVf("[%d] No port specified, using %s", bc.id, bc.port)
	}
	var addr net.Addr
	if o.UnixDomainSocket != "" {
		log.Infof("[%d] Using unix domain socket %v instead of %v %v", bc.id, o.UnixDomainSocket, bc.hostname, bc.port)
		uds := &net.UnixAddr{Name: o.UnixDomainSocket, Net: fnet.UnixDomainSocket}
		addr = uds
	} else {
		var tAddr *net.TCPAddr // strangely we get a non nil wrap of nil if assigning to addr directly
		var err error
		tAddr, err = resolve(bc.hostname, bc.port, o.Resolve, bc.ipAddrUsage)
		if tAddr == nil {
			// Error already logged
			return nil, err
		}
		addr = tAddr
	}
	bc.dest = addr
	// Create the bytes for the request:
	host := bc.host
	customHostHeader := (o.hostOverride != "")
	if customHostHeader {
		host = o.hostOverride
	}
	if bc.tlsConfig != nil {
		bc.tlsConfig.ServerName = bc.hostname // Shouldn't have a port #571
	}
	var buf bytes.Buffer
	buf.WriteString(method + " " + url.RequestURI() + " HTTP/" + proto + "\r\n")
	if !bc.http10 || customHostHeader {
		buf.WriteString("Host: " + host + "\r\n")
	}
	if !bc.http10 {
		// Rest of normal http 1.1 processing:
		bc.parseHeaders = true
		if !o.DisableKeepAlive {
			bc.keepAlive = true
		} else {
			buf.WriteString("Connection: close\r\n")
		}
	}
	bc.reqTimeout = o.HTTPReqTimeOut
	w := bufio.NewWriter(&buf)
	// This writes multiple valued headers properly (unlike calling Get() to do it ourselves)
	_ = o.GenerateHeaders().Write(w)
	w.Flush()
	buf.WriteString("\r\n")
	// Add the payload to http body
	if payloadLen > 0 {
		buf.Write(o.Payload)
	}
	bc.req = buf.Bytes()
	bc.uuidMarkers = [][]byte{}
	if len(uuidStrings) > 0 {
		for _, uuidString := range uuidStrings {
			bc.uuidMarkers = append(bc.uuidMarkers, []byte(uuidString))
		}
	}
	log.Debugf("[%d] Created client:\n%+v\n%s", bc.id, bc.dest, bc.req)
	return &bc, nil
}

// return the result from the state.
func (c *FastClient) returnRes() (int, []byte, int) {
	return c.code, c.buffer[:c.size], c.headerLen
}

// connect to destination.
func (c *FastClient) connect() net.Conn {
	c.socketCount++
	var socket net.Conn
	var err error

	// Resolve the DNS name when making new connections.
	if c.socketCount > 1 && !c.noResolveEachConn {
		c.dest, err = resolve(c.hostname, c.port, c.resolve, c.ipAddrUsage)
		log.Debugf("[%d] Hostname %v resolve to ip %v", c.id, c.hostname, c.dest)
		if err != nil {
			log.Errf("[%d] Unable to resolve hostname %v: %v", c.id, c.hostname, err)
			return nil
		}
	}

	d := &net.Dialer{Timeout: c.reqTimeout}
	if c.https {
		socket, err = tls.DialWithDialer(d, c.dest.Network(), c.dest.String(), c.tlsConfig)
		if err != nil {
			log.Errf("[%d] Unable to TLS connect to %v : %v", c.id, c.dest, err)
			return nil
		}
	} else {
		socket, err = d.Dial(c.dest.Network(), c.dest.String())
		if err != nil {
			log.Errf("[%d] Unable to connect to %v : %v", c.id, c.dest, err)
			return nil
		}
	}
	fnet.SetSocketBuffers(socket, len(c.buffer), len(c.req))
	return socket
}

// Extra error codes outside of the HTTP Status code ranges. ie negative.
const (
	// SocketError is return when a transport error occurred: unexpected EOF, connection error, etc...
	SocketError = -1
	// RetryOnce is used internally as an error code to allow 1 retry for bad socket reuse.
	RetryOnce = -2
)

// Fetch fetches the url content. Returns http code, data, offset of body.
func (c *FastClient) Fetch() (int, []byte, int) {
	c.code = SocketError
	c.size = 0
	c.headerLen = 0
	// Connect or reuse existing socket:
	conn := c.socket
	canReuse := conn != nil
	if c.reachedReuseThreshold() {
		c.connReuse = generateReuseThreshold(c.connReuseRange[0], c.connReuseRange[1])
		log.LogVf("[%d] Thread reach the threshold for max connection canReuse of %d, force create new connection",
			c.id, c.connReuse)
	}

	if conn == nil {
		conn = c.connect()
		c.reuseCount = 1
		if conn == nil {
			return c.returnRes()
		}
	} else {
		c.reuseCount++
		log.Debugf("[%d] Reusing socket %v", c.id, c.dest)
	}
	c.socket = nil // because of error returns and single retry
	conErr := conn.SetDeadline(time.Now().Add(c.reqTimeout))
	// Send the request:
	req := c.req
	if len(c.uuidMarkers) > 0 {
		for _, uuidMarker := range c.uuidMarkers {
			req = bytes.Replace(req, uuidMarker, []byte(generateUUID()), 1)
		}
	}
	n, err := conn.Write(req)
	if err != nil || conErr != nil {
		if canReuse {
			// it's ok for the (idle) socket to die once, auto reconnect:
			log.Infof("[%d] Closing dead socket %v (%v)", c.id, c.dest, err)
			conn.Close()
			c.errorCount++
			return c.Fetch() // recurse once
		}
		log.Errf("[%d] Unable to write to %v : %v", c.id, c.dest, err)
		return c.returnRes()
	}
	if n != len(c.req) {
		log.Errf("[%d] Short write to %v : %d instead of %d", c.id, c.dest, n, len(c.req))
		return c.returnRes()
	}
	if !c.keepAlive && c.halfClose { // nolint: nestif
		tcpConn, ok := conn.(*net.TCPConn)
		if ok {
			if err = tcpConn.CloseWrite(); err != nil {
				log.Errf("[%d] Unable to close write to %v : %v", c.id, c.dest, err)
				return c.returnRes()
			} // else:
			log.Debugf("[%d] Half closed ok after sending request %v", c.id, c.dest)
		} else {
			log.Warnf("[%d] Unable to close write non tcp connection %v", c.id, c.dest)
		}
	}
	// Read the response:
	c.readResponse(conn, canReuse)
	if c.code == RetryOnce {
		// Special "eof on reused socket" code
		return c.Fetch() // recurse once
	}
	// Return the result:
	return c.returnRes()
}

func codeIsOK(code int) bool {
	// TODO: make this configurable
	return (code >= 200 && code <= 299) || code == http.StatusTeapot
}

// Response reading:
// nolint: nestif,funlen,gocognit,gocyclo,maintidx // TODO: refactor - unwiedly/ugly atm.
func (c *FastClient) readResponse(conn net.Conn, reusedSocket bool) {
	max := len(c.buffer)
	parsedHeaders := false
	// TODO: safer to start with -1 / SocketError and fix ok for http 1.0
	c.code = http.StatusOK // In http 1.0 mode we don't bother parsing anything
	endofHeadersStart := retcodeOffset + 3
	keepAlive := c.keepAlive
	chunkedMode := false
	checkConnectionClosedHeader := CheckConnectionClosedHeader
	skipRead := false
	for {
		// Ugly way to cover the case where we get more than 1 chunk at the end
		// TODO: need automated tests
		if !skipRead {
			n, err := conn.Read(c.buffer[c.size:])
			if err != nil {
				if reusedSocket && c.size == 0 {
					// Ok for reused socket to be dead once (close by server)
					log.Infof("[%d] Closing dead socket %v (err %v at first read)", c.id, c.dest, err)
					c.errorCount++
					err = conn.Close() // close the previous one
					if err != nil {
						log.Warnf("[%d] Error closing dead socket %v: %v", c.id, c.dest, err)
					}
					c.code = RetryOnce // special "retry once" code
					return
				}
				if errors.Is(err, io.EOF) && c.size != 0 {
					// handled below as possibly normal end of stream after we read something
					break
				}
				log.Errf("[%d] Read error for %v %d : %v", c.id, c.dest, c.size, err)
				c.code = SocketError
				break
			}
			c.size += n
			if log.LogDebug() {
				log.Debugf("[%d] Read ok %d total %d so far (-%d headers = %d data) %s",
					c.id, n, c.size, c.headerLen, c.size-c.headerLen, DebugSummary(c.buffer[c.size-n:c.size], 256))
			}
		}
		skipRead = false
		// Have not yet parsed the headers, need to parse the headers, and have enough data to
		// at least parse the http retcode:
		if !parsedHeaders && c.parseHeaders && c.size >= retcodeOffset+3 {
			// even if the bytes are garbage we'll get a non 200 code (bytes are unsigned)
			c.code = ParseDecimal(c.buffer[retcodeOffset : retcodeOffset+3]) // TODO do that only once...
			// TODO handle 100 Continue, make the "ok" codes configurable
			if !codeIsOK(c.code) {
				if c.logErrors {
					log.Warnf("[%d] Non ok http code %d (%v)", c.id, c.code, string(c.buffer[:retcodeOffset+3]))
				}
				break
			}
			if log.LogDebug() {
				log.Debugf("[%d] Code %d, looking for end of headers at %d / %d, last CRLF %d",
					c.id, c.code, endofHeadersStart, c.size, c.headerLen)
			}
			// TODO: keep track of list of newlines to efficiently search headers only there
			idx := endofHeadersStart
			for idx < c.size-1 {
				if c.buffer[idx] == '\r' && c.buffer[idx+1] == '\n' {
					if c.headerLen == idx-2 { // found end of headers
						parsedHeaders = true
						break
					}
					c.headerLen = idx
					idx++
				}
				idx++
			}
			endofHeadersStart = c.size // start there next read
			if parsedHeaders {
				// We have headers !
				c.headerLen += 4 // we use this and not endofHeadersStart so http/1.0 does return 0 and not the optimization for search start
				if log.LogDebug() {
					log.Debugf("[%d] headers are %d: %q", c.id, c.headerLen, c.buffer[:idx])
				}
				// Find the content length or chunked mode
				if keepAlive {
					var contentLength int
					found, offset := FoldFind(c.buffer[:c.headerLen], contentLengthHeader)
					if found {
						// Content-Length mode:
						contentLength = ParseDecimal(c.buffer[offset+len(contentLengthHeader) : c.headerLen])
						if contentLength < 0 {
							log.Warnf("[%d] Warning: content-length unparsable %s", c.id, string(c.buffer[offset+2:offset+len(contentLengthHeader)+4]))
							keepAlive = false
							break
						}
						max = c.headerLen + contentLength
						if log.LogDebug() { // somehow without the if we spend 400ms/10s in LogV (!)
							log.Debugf("[%d] found content length %d", c.id, contentLength)
						}
					} else {
						// Chunked mode (or err/missing):
						if found, _ := FoldFind(c.buffer[:c.headerLen], chunkedHeader); found {
							chunkedMode = true
							var dataStart int
							dataStart, contentLength = ParseChunkSize(c.buffer[c.headerLen:c.size])
							if contentLength == -1 {
								// chunk length not available yet
								log.LogVf("[%d] chunk mode but no first chunk length yet, reading more", c.id)
								max = c.headerLen
								continue
							}
							max = c.headerLen + dataStart + contentLength + 2 // extra CR LF
							log.Debugf("[%d] chunk-length is %d (%s) setting max to %d",
								c.id, contentLength, c.buffer[c.headerLen:c.headerLen+dataStart-2],
								max)
						} else {
							if log.LogVerbose() {
								log.LogVf("[%d] Warning: content-length missing in %q", c.id, string(c.buffer[:c.headerLen]))
							} else {
								log.Warnf("[%d] Warning: content-length missing (%d bytes headers)", c.id, c.headerLen)
							}
							keepAlive = false // can't keep keepAlive
							break
						}
					} // end of content-length section
					if max > len(c.buffer) {
						log.Warnf("[%d] Buffer is too small for headers %d + data %d - change -httpbufferkb flag to at least %d",
							c.id, c.headerLen, contentLength, (c.headerLen+contentLength)/1024+1)
						// TODO: just consume the extra instead
						max = len(c.buffer)
					}
					if checkConnectionClosedHeader {
						if found, _ := FoldFind(c.buffer[:c.headerLen], connectionCloseHeader); found {
							log.Infof("[%d] Server wants to close connection, no keep-alive!", c.id)
							keepAlive = false
							max = len(c.buffer) // reset to read as much as available
						}
					}
				}
			}
		} // end of big if parse header
		if c.size >= max {
			if !keepAlive {
				log.Errf("[%d] More data is available but stopping after %d, increase -httpbufferkb", c.id, max)
			}
			if !parsedHeaders && c.parseHeaders {
				log.Errf("[%d] Buffer too small (%d) to even finish reading headers, increase -httpbufferkb to get all the data", c.id, max)
				keepAlive = false
			}
			if chunkedMode {
				// Next chunk:
				dataStart, nextChunkLen := ParseChunkSize(c.buffer[max:c.size])
				if nextChunkLen == -1 {
					if c.size == max {
						log.Debugf("[%d] Couldn't find next chunk size, reading more %d %d", c.id, max, c.size)
					} else {
						log.Infof("[%d] Partial chunk size (%s), reading more %d %d", c.id, DebugSummary(c.buffer[max:c.size], 20), max, c.size)
					}
					continue
				} else if nextChunkLen == 0 {
					log.Debugf("[%d] Found last chunk %d %d", c.id, max+dataStart, c.size)
					if c.size != max+dataStart+2 || string(c.buffer[c.size-2:c.size]) != "\r\n" {
						log.Errf("[%d] Unexpected mismatch at the end sz=%d expected %d; end of buffer %q",
							c.id, c.size, max+dataStart+2, c.buffer[max:c.size])
					}
				} else {
					max += dataStart + nextChunkLen + 2 // extra CR LF
					log.Debugf("[%d] One more chunk %d -> new max %d", c.id, nextChunkLen, max)
					if max > len(c.buffer) {
						log.Errf("[%d] Buffer too small for %d data", c.id, max)
					} else {
						if max <= c.size {
							log.Debugf("[%d] Enough data to reach next chunk, skipping a read", c.id)
							skipRead = true
						}
						continue
					}
				}
			}
			break // we're done!
		}
	} // end of big for loop
	// Figure out whether to keep or close the socket:
	if keepAlive && codeIsOK(c.code) && !c.reachedReuseThreshold() {
		c.socket = conn // keep the open socket
	} else {
		if err := conn.Close(); err != nil {
			log.Errf("[%d] Close error %v %d : %v", c.id, c.dest, c.size, err)
		} else {
			log.Debugf("[%d] Closed ok from %v after reading %d bytes", c.id, c.dest, c.size)
		}
		// we cleared c.socket in caller already
	}
}

// Check if current thread reached the connection reuse threshold.
func (c *FastClient) reachedReuseThreshold() bool {
	if c.connReuse != 0 && c.reuseCount >= c.connReuse {
		log.LogVf("[%d] Thread reach the threshold for max connection reuse of %d, force create new connection",
			c.id, c.connReuse)
		return true
	}

	return false
}

func generateUUID() string {
	// We use math random instead of crypto random generator due to performance.
	return uuid.Must(uuid.NewRandomFromReader(rander)).String()
}

// Generate reuse threshold based on the min and max value in the flag.
func generateReuseThreshold(min int, max int) int {
	if min == max {
		return min
	}

	return min + rand.Intn(max-min+1) // nolint: gosec // we want fast not crypto
}

// Resolve the DNS hostname to ip address or assign the override IP.
func resolve(hostname string, port string, overrideIp string, ipAddrUsage *stats.Occurrence) (*net.TCPAddr, error) {
	var addr *net.TCPAddr
	var err error
	if overrideIp != "" {
		addr, err = fnet.Resolve(overrideIp, port)
	} else {
		addr, err = fnet.Resolve(hostname, port)
	}

	ipAddrUsage.Record(addr.String())

	return addr, err
}
