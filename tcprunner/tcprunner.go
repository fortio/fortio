// Copyright 2020 Fortio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     tcp://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tcprunner

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"sort"
	"strings"
	"time"

	"fortio.org/fortio/fhttp"
	"fortio.org/fortio/fnet"
	"fortio.org/fortio/log"
	"fortio.org/fortio/periodic"
)

type TCPResultMap map[string]int64

// RunnerResults is the aggregated result of an TCPRunner.
// Also is the internal type used per thread/goroutine.
type RunnerResults struct {
	periodic.RunnerResults
	TCPOptions
	RetCodes      TCPResultMap
	SocketCount   int
	BytesSent     int64
	BytesReceived int64
	client        *TCPClient
	aborter       *periodic.Aborter
}

// Run tests tcp request fetching. Main call being run at the target QPS.
// To be set as the Function in RunnerOptions.
func (tcpstate *RunnerResults) Run(t int) {
	log.Debugf("Calling in %d", t)
	_, err := tcpstate.client.Fetch()
	if err != nil {
		tcpstate.RetCodes[err.Error()]++
	} else {
		tcpstate.RetCodes[TCPStatusOK]++
	}
}

// TCPOptions are options to the TCPClient.
type TCPOptions struct {
	Destination      string
	Payload          []byte // what to send (and check)
	UnixDomainSocket string // Path of unix domain socket to use instead of host:port from URL
	ReqTimeout       time.Duration
}

// RunnerOptions includes the base RunnerOptions plus tcp specific
// options.
type RunnerOptions struct {
	periodic.RunnerOptions
	TCPOptions // Need to call Init() to initialize
}

// TCPClient is the client used for tcp echo testing.
type TCPClient struct {
	buffer        []byte
	req           []byte
	dest          net.Addr
	socket        net.Conn
	connID        int // 0-9999
	messageCount  int64
	bytesSent     int64
	bytesReceived int64
	socketCount   int
	destination   string
	doGenerate    bool
	reqTimeout    time.Duration
}

var (
	// TCPURLPrefix is the URL prefix for triggering tcp load.
	TCPURLPrefix = "tcp://"
	// TCPStatusOK is the map key on success.
	TCPStatusOK  = "OK"
	errShortRead = fmt.Errorf("short read")
	errLongRead  = fmt.Errorf("bug: long read")
	errMismatch  = fmt.Errorf("read not echoing writes")
)

// Generates a 24 bytes unique payload for each runner thread and message sent.
func GeneratePayload(t int, i int64) []byte {
	// up to 9999 connections and 999 999 999 999 (999B) request
	s := fmt.Sprintf("Fortio\n%04d\n%012d", t, i) // 6+2+4+12 = 24 bytes
	return []byte(s)
}

// NewTCPClient creates and initialize and returns a client based on the TCPOptions.
func NewTCPClient(o *TCPOptions) (*TCPClient, error) {
	c := TCPClient{}
	d := o.Destination
	d = strings.TrimPrefix(d, TCPURLPrefix)
	d = strings.TrimSuffix(d, "/")
	c.destination = d
	tAddr, err := fnet.ResolveDestination(d)
	if tAddr == nil {
		return nil, err
	}
	c.dest = tAddr
	c.req = o.Payload
	if len(c.req) == 0 { // len(nil) array is also valid and 0
		c.doGenerate = true
		c.req = GeneratePayload(0, 0)
	}
	c.buffer = make([]byte, len(c.req))
	c.reqTimeout = o.ReqTimeout
	if o.ReqTimeout == 0 {
		log.Debugf("Request timeout not set, using default %v", fhttp.HTTPReqTimeOutDefaultValue)
		c.reqTimeout = fhttp.HTTPReqTimeOutDefaultValue
	}
	if c.reqTimeout < 0 {
		log.Warnf("Invalid timeout %v, setting to %v", c.reqTimeout, fhttp.HTTPReqTimeOutDefaultValue)
		c.reqTimeout = fhttp.HTTPReqTimeOutDefaultValue
	}
	return &c, nil
}

func (c *TCPClient) connect() (net.Conn, error) {
	c.socketCount++
	socket, err := net.Dial(c.dest.Network(), c.dest.String())
	if err != nil {
		log.Errf("Unable to connect to %v : %v", c.dest, err)
		return nil, err
	}
	fnet.SetSocketBuffers(socket, len(c.buffer), len(c.req))
	return socket, nil
}

func (c *TCPClient) Fetch() ([]byte, error) {
	// Connect or reuse existing socket:
	conn := c.socket
	c.messageCount++
	reuse := (conn != nil)
	if !reuse {
		var err error
		conn, err = c.connect()
		if conn == nil {
			return nil, err
		}
	} else {
		log.Debugf("Reusing socket %v", conn)
	}
	c.socket = nil // because of error returns and single retry
	conErr := conn.SetReadDeadline(time.Now().Add(c.reqTimeout))
	// Send the request:
	if c.doGenerate {
		c.req = GeneratePayload(c.connID, c.messageCount) // TODO write directly in buffer to avoid generating garbage for GC to clean
	}
	n, err := conn.Write(c.req)
	c.bytesSent = c.bytesSent + int64(n)
	if log.LogDebug() {
		log.Debugf("wrote %d (%q): %v", n, string(c.req), err)
	}
	if err != nil || conErr != nil {
		if reuse {
			// it's ok for the (idle) socket to die once, auto reconnect:
			log.Infof("Closing dead socket %v (%v)", conn, err)
			conn.Close()
			return c.Fetch() // recurse once
		}
		log.Errf("Unable to write to %v %v : %v", conn, c.dest, err)
		return nil, err
	}
	if n != len(c.req) {
		log.Errf("Short write to %v %v : %d instead of %d", conn, c.dest, n, len(c.req))
		return nil, io.ErrShortWrite
	}
	// assert that len(c.buffer) == len(c.req)
	n, err = conn.Read(c.buffer)
	c.bytesReceived = c.bytesReceived + int64(n)
	if log.LogDebug() {
		log.Debugf("read %d (%q): %v", n, string(c.buffer[:n]), err)
	}
	if n < len(c.req) {
		return c.buffer[:n], errShortRead
	}
	if n > len(c.req) {
		log.Errf("BUG: read more than possible %d vs %d", n, len(c.req))
		return c.buffer[:n], errLongRead
	}
	if !bytes.Equal(c.buffer, c.req) {
		log.Infof("Mismatch between sent %q and received %q", string(c.req), string(c.buffer))
		return c.buffer, errMismatch
	}
	c.socket = conn // reuse on success
	return c.buffer[:n], nil
}

// Close closes the last connection and returns the total number of sockets used for the run.
func (c *TCPClient) Close() int {
	log.Debugf("Closing %p: %s socket count %d", c, c.destination, c.socketCount)
	if c.socket != nil {
		if err := c.socket.Close(); err != nil {
			log.Warnf("Error closing tcp client's socket: %v", err)
		}
		c.socket = nil
	}
	return c.socketCount
}

// RunTCPTest runs an tcp test and returns the aggregated stats.
// Some refactoring to avoid copy-pasta between the now 3 runners would be good.
func RunTCPTest(o *RunnerOptions) (*RunnerResults, error) {
	o.RunType = "TCP"
	log.Infof("Starting tcp test for %s with %d threads at %.1f qps", o.Destination, o.NumThreads, o.QPS)
	r := periodic.NewPeriodicRunner(&o.RunnerOptions)
	defer r.Options().Abort()
	numThreads := r.Options().NumThreads
	o.TCPOptions.Destination = o.Destination
	out := r.Options().Out // Important as the default value is set from nil to stdout inside NewPeriodicRunner
	total := RunnerResults{
		aborter:  r.Options().Stop,
		RetCodes: make(TCPResultMap),
	}
	total.Destination = o.Destination
	tcpstate := make([]RunnerResults, numThreads)
	var err error
	for i := 0; i < numThreads; i++ {
		r.Options().Runners[i] = &tcpstate[i]
		// Create a client (and transport) and connect once for each 'thread'
		tcpstate[i].client, err = NewTCPClient(&o.TCPOptions)
		if tcpstate[i].client == nil {
			return nil, fmt.Errorf("unable to create client %d for %s: %w", i, o.Destination, err)
		}
		tcpstate[i].client.connID = i
		if o.Exactly <= 0 {
			data, err := tcpstate[i].client.Fetch()
			if i == 0 && log.LogVerbose() {
				log.LogVf("first hit of %s: err %v, received %d: %q", o.Destination, err, len(data), data)
			}
		}
		// Setup the stats for each 'thread'
		tcpstate[i].aborter = total.aborter
		tcpstate[i].RetCodes = make(TCPResultMap)
	}
	total.RunnerResults = r.Run()
	// Numthreads may have reduced but it should be ok to accumulate 0s from
	// unused ones. We also must cleanup all the created clients.
	keys := []string{}
	for i := 0; i < numThreads; i++ {
		total.SocketCount += tcpstate[i].client.Close()
		total.BytesReceived += tcpstate[i].client.bytesReceived
		total.BytesSent += tcpstate[i].client.bytesSent
		for k := range tcpstate[i].RetCodes {
			if _, exists := total.RetCodes[k]; !exists {
				keys = append(keys, k)
			}
			total.RetCodes[k] += tcpstate[i].RetCodes[k]
		}
	}
	// Cleanup state:
	r.Options().ReleaseRunners()
	totalCount := float64(total.DurationHistogram.Count)
	_, _ = fmt.Fprintf(out, "Sockets used: %d (for perfect no error run, would be %d)\n", total.SocketCount, r.Options().NumThreads)
	_, _ = fmt.Fprintf(out, "Total Bytes sent: %d, received: %d\n", total.BytesSent, total.BytesReceived)
	sort.Strings(keys)
	for _, k := range keys {
		_, _ = fmt.Fprintf(out, "tcp %s : %d (%.1f %%)\n", k, total.RetCodes[k], 100.*float64(total.RetCodes[k])/totalCount)
	}
	return &total, nil
}
