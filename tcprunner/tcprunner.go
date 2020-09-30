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
	"fmt"
	"io"
	"net"
	"sort"
	"strings"
	"time"

	"fortio.org/fortio/fnet"
	"fortio.org/fortio/log"
	"fortio.org/fortio/periodic"
)

type TCPResultMap map[string]int64

var TCPStatusOK = "OK"

// TCPRunnerResults is the aggregated result of an TCPRunner.
// Also is the internal type used per thread/goroutine.
type TCPRunnerResults struct {
	periodic.RunnerResults
	TCPOptions
	RetCodes    TCPResultMap
	SocketCount int
	client      *TCPClient
	aborter     *periodic.Aborter
}

// Run tests tcp request fetching. Main call being run at the target QPS.
// To be set as the Function in RunnerOptions.
func (tcpstate *TCPRunnerResults) Run(t int) {
	log.Debugf("Calling in %d", t)
	_, err := tcpstate.client.Fetch()
	if err != nil {
		tcpstate.RetCodes[err.Error()]++
	} else {
		tcpstate.RetCodes[TCPStatusOK]++
	}
}

type TCPOptions struct {
	Destination      string
	Payload          []byte // what to send (and check)
	UnixDomainSocket string // Path of unix domain socket to use instead of host:port from URL
}

// TCPRunnerOptions includes the base RunnerOptions plus tcp specific
// options.
type TCPRunnerOptions struct {
	periodic.RunnerOptions
	TCPOptions // Need to call Init() to initialize
}

type TCPClient struct {
	buffer      []byte
	req         []byte
	dest        net.Addr
	socket      net.Conn
	socketCount int
	destination string
	reqTimeout  time.Duration
}

var TCPURLPrefix = "tcp://"

func NewTCPClient(o *TCPOptions) *TCPClient {
	c := TCPClient{}
	d := o.Destination
	d = strings.TrimPrefix(d, TCPURLPrefix)
	d = strings.TrimSuffix(d, "/")
	c.destination = d
	tAddr := fnet.ResolveDestination(d)
	if tAddr == nil {
		return nil
	}
	c.dest = tAddr
	c.req = o.Payload
	if c.req == nil {
		c.req = []byte("Fortio!\n") // 8 bytes
	}
	c.buffer = make([]byte, len(c.req))
	return &c
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
	n, err := conn.Write(c.req)
	log.Debugf("wrote %d on %v: %v", n, c.req, err)
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
	return nil, nil
}

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
func RunTCPTest(o *TCPRunnerOptions) (*TCPRunnerResults, error) {
	o.RunType = "TCP"
	log.Infof("Starting tcp test for %s with %d threads at %.1f qps", o.Destination, o.NumThreads, o.QPS)
	r := periodic.NewPeriodicRunner(&o.RunnerOptions)
	defer r.Options().Abort()
	numThreads := r.Options().NumThreads
	o.TCPOptions.Destination = o.Destination
	out := r.Options().Out // Important as the default value is set from nil to stdout inside NewPeriodicRunner
	total := TCPRunnerResults{
		aborter:  r.Options().Stop,
		RetCodes: make(TCPResultMap),
	}
	total.Destination = o.Destination
	tcpstate := make([]TCPRunnerResults, numThreads)
	for i := 0; i < numThreads; i++ {
		r.Options().Runners[i] = &tcpstate[i]
		// Create a client (and transport) and connect once for each 'thread'
		tcpstate[i].client = NewTCPClient(&o.TCPOptions)
		if tcpstate[i].client == nil {
			return nil, fmt.Errorf("unable to create client %d for %s", i, o.Destination)
		}
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
	_, _ = fmt.Fprintf(out, "Sockets used: %d (for perfect keepalive, would be %d)\n", total.SocketCount, r.Options().NumThreads)
	sort.Strings(keys)
	for _, k := range keys {
		_, _ = fmt.Fprintf(out, "tcp %s : %d (%.1f %%)\n", k, total.RetCodes[k], 100.*float64(total.RetCodes[k])/totalCount)
	}
	return &total, nil
}
