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

package fnet_test

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"fortio.org/fortio/fnet"
	"fortio.org/fortio/log"
	"fortio.org/fortio/version"
)

func TestNormalizePort(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		output string
	}{
		{
			"port number only",
			"8080",
			":8080",
		},
		{
			"IPv4 host:port",
			"10.10.10.1:8080",
			"10.10.10.1:8080",
		},
		{
			"IPv6 [host]:port",
			"[2001:db1::1]:8080",
			"[2001:db1::1]:8080",
		},
	}

	for _, tc := range tests {
		port := fnet.NormalizePort(tc.input)
		if port != tc.output {
			t.Errorf("Test case %s failed to normailze port %s\n\texpected: %s\n\t  actual: %s",
				tc.name,
				tc.input,
				tc.output,
				port,
			)
		}
	}
}

func TestListen(t *testing.T) {
	l, a := fnet.Listen("test listen1", "0")
	if l == nil || a == nil {
		t.Fatalf("Unexpected nil in fnet.Listen() %v %v", l, a)
	}
	if a.(*net.TCPAddr).Port == 0 {
		t.Errorf("Unexpected 0 port after listen %+v", a)
	}
	_ = l.Close()
}

func TestListenFailure(t *testing.T) {
	_, a1 := fnet.Listen("test listen2", "0")
	if a1.(*net.TCPAddr).Port == 0 {
		t.Errorf("Unexpected 0 port after listen %+v", a1)
	}
	l, a := fnet.Listen("this should fail", fnet.GetPort(a1))
	if l != nil || a != nil {
		t.Errorf("listen that should error got %v %v instead of nil", l, a)
	}
}

func TestUDPListenFailure(t *testing.T) {
	_, a1 := fnet.UDPListen("test listen2", "0")
	if a1.(*net.UDPAddr).Port == 0 {
		t.Errorf("Unexpected 0 port after listen %+v", a1)
	}
	l, a := fnet.UDPListen("this should fail", fnet.GetPort(a1))
	if l != nil || a != nil {
		t.Errorf("udp listen that should error got %v %v instead of nil", l, a)
	}
	l, a = fnet.UDPListen("this should fail", ":doesnotexisthopefully")
	if l != nil || a != nil {
		t.Errorf("udp listen with bogus port should error got %v %v instead of nil", l, a)
	}
}

func TestResolveDestination(t *testing.T) {
	tests := []struct {
		name        string
		destination string
		want        string
	}{
		// Error cases:
		{"missing :", "foo", ""},
		{"using ip:bogussvc", "8.8.8.8:doesnotexisthopefully", ""},
		{"using bogus hostname", "doesnotexist.fortio.org:443", ""},
		{"using udp://ip:portname", "udp://8.8.8.8:http", ""},
		// Good cases:
		{"using tcp://ip:portname", "tcp://8.8.8.8:http", "8.8.8.8:80"},
		{"using tcp://ip:portname/", "tcp://8.8.8.8:http/", "8.8.8.8:80"},
		{"using ip:portname", "8.8.8.8:http", "8.8.8.8:80"},
		{"using ip:port", "8.8.8.8:12345", "8.8.8.8:12345"},
		{"using [ipv6]:port", "[::1]:12345", "[::1]:12345"},
	}
	for _, tt := range tests {
		tt := tt // pin
		t.Run(tt.name, func(t *testing.T) {
			got, _ := fnet.TCPResolveDestination(tt.destination)
			gotStr := ""
			if got != nil {
				gotStr = got.String()
			}
			if gotStr != tt.want {
				t.Errorf("ResolveDestination(%s) = %v, want %s", tt.destination, got, tt.want)
			}
		})
	}
}

func TestUDPResolveDestination(t *testing.T) {
	tests := []struct {
		name        string
		destination string
		want        string
	}{
		// Error cases:
		{"missing :", "foo", ""},
		{"using ip:bogussvc", "8.8.8.8:doesnotexisthopefully", ""},
		{"using bogus hostname", "doesnotexist.fortio.org:443", ""},
		{"using tcp://ip:portname", "tcp://8.8.8.8:domain", ""},
		// Good cases:
		{"using udp://ip:portname", "udp://8.8.8.8:domain", "8.8.8.8:53"},
		{"using udp://ip:portname/", "udp://8.8.8.8:domain/", "8.8.8.8:53"},
		{"using ip:portname", "8.8.8.8:domain", "8.8.8.8:53"},
		{"using ip:port", "8.8.8.8:12345", "8.8.8.8:12345"},
		{"using [ipv6]:port", "[::1]:12345", "[::1]:12345"},
	}
	for _, tt := range tests {
		tt := tt // pin
		t.Run(tt.name, func(t *testing.T) {
			got, _ := fnet.UDPResolveDestination(tt.destination)
			gotStr := ""
			if got != nil {
				gotStr = got.String()
			}
			if gotStr != tt.want {
				t.Errorf("ResolveDestination(%s) = %v, want %s", tt.destination, got, tt.want)
			}
		})
	}
}

func TestResolveDestinationMultipleIps(t *testing.T) {
	addr, err := fnet.TCPResolveDestination("www.google.com:443")
	t.Logf("Found google addr %+v err=%v", addr, err)
	if addr == nil || err != nil {
		t.Errorf("got nil address for google: %v", err)
	}
}

func TestProxy(t *testing.T) {
	addr := fnet.ProxyToDestination(":0", "www.google.com:80")
	dAddr := net.TCPAddr{Port: addr.(*net.TCPAddr).Port}
	d, err := net.DialTCP("tcp", nil, &dAddr)
	if err != nil {
		t.Fatalf("can't connect to our proxy: %v", err)
	}
	defer d.Close()
	data := "HEAD / HTTP/1.0\r\nUser-Agent: fortio-unit-test-" + version.Long() + "\r\n\r\n"
	_, _ = d.Write([]byte(data))
	_ = d.CloseWrite()
	res := make([]byte, 4096)
	n, err := d.Read(res)
	if err != nil {
		t.Errorf("read error with proxy: %v", err)
	}
	resStr := string(res[:n])
	expectedStart := "HTTP/1.0 200 OK\r\n"
	if !strings.HasPrefix(resStr, expectedStart) {
		t.Errorf("Unexpected reply '%q', expected starting with '%q'", resStr, expectedStart)
	}
}

func TestTcpEcho(t *testing.T) {
	addr := fnet.TCPEchoServer("test-tcp-echo", ":0")
	dAddr := net.TCPAddr{Port: addr.(*net.TCPAddr).Port}
	d, err := net.DialTCP("tcp", nil, &dAddr)
	if err != nil {
		t.Fatalf("can't connect to our echo server: %v", err)
	}
	defer d.Close()
	data := "F\000oBar\000\001"
	_, _ = d.Write([]byte(data))
	_ = d.CloseWrite()
	res := make([]byte, 4096)
	n, err := d.Read(res)
	if err != nil {
		t.Errorf("read error with proxy: %v", err)
	}
	resStr := string(res[:n])
	if resStr != data {
		t.Errorf("Unexpected echo '%q', expected what we sent: '%q'", resStr, data)
	}
}

func TestUdpEcho(t *testing.T) {
	for i := 0; i <= 1; i++ {
		async := (i == 0)
		addr := fnet.UDPEchoServer("test-udp-echo", ":0", async)
		port := addr.(*net.UDPAddr).Port
		in := ioutil.NopCloser(strings.NewReader("ABCDEF"))
		var buf bytes.Buffer
		dest := fmt.Sprintf("udp://localhost:%d", port)
		out := bufio.NewWriter(&buf)
		err := fnet.NetCat(dest, in, out, true)
		if err != nil {
			t.Errorf("Unexpected NetCat err: %v", err)
		}
		out.Flush()
		res := buf.String()
		if res != "ABCDEF" {
			t.Errorf("Got unexpected %q", res)
		}
	}
}

type ErroringWriter struct{}

func (cbb *ErroringWriter) Close() error {
	return nil
}

func (cbb *ErroringWriter) Write(buf []byte) (int, error) {
	return len(buf) / 2, io.ErrClosedPipe
}

// Also tests NetCat and copy.
func TestTCPEchoServerErrors(t *testing.T) {
	addr := fnet.TCPEchoServer("test-tcp-echo", ":0")
	dAddr := net.TCPAddr{Port: addr.(*net.TCPAddr).Port}
	port := dAddr.String()
	log.Infof("Connecting to %q", port)
	log.SetLogLevel(log.Verbose)
	// 2nd proxy on same port should fail
	addr2 := fnet.TCPEchoServer("test-tcp-echo-error", fnet.GetPort(addr))
	if addr2 != nil {
		t.Errorf("Second proxy on same port should have failed, got %+v", addr2)
	}
	// For some reason unable to trigger these 2 cases within go
	// TODO: figure it out... this is now only triggering coverage but not really testing anything
	// quite brittle but somehow we can get read: connection reset by peer and write: broken pipe
	// with these timings (!)
	eofStopFlag := false
	for i := 0; i < 2; i++ {
		in := ioutil.NopCloser(strings.NewReader(strings.Repeat("x", 50000)))
		var out ErroringWriter
		fnet.NetCat("localhost"+port, in, &out, eofStopFlag)
		eofStopFlag = true
	}
}

func TestNetCatErrors(t *testing.T) {
	listener, addr := fnet.Listen("test-closed-listener", ":0")
	dAddr := net.TCPAddr{Port: addr.(*net.TCPAddr).Port}
	listener.Close()
	err := fnet.NetCat("localhost"+dAddr.String(), nil, nil, true)
	if err == nil {
		t.Errorf("Expected connect error on closed server, got success")
	}
}

func TestSetSocketBuffersError(t *testing.T) {
	c := &net.UnixConn{}
	fnet.SetSocketBuffers(c, 512, 256) // triggers 22:11:14 V network.go:245> Not setting socket options on non tcp socket <nil>
}

func TestSmallReadUntil(t *testing.T) {
	d, err := net.Dial("tcp", "www.google.com:80")
	if err != nil {
		t.Fatalf("can't connect to google to test: %v", err)
	}
	defer d.Close()
	data := "HEAD / HTTP/1.0\r\nUser-Agent: fortio-unit-test-" + version.Long() + "\r\n\r\n"
	_, _ = d.Write([]byte(data))
	_ = d.(*net.TCPConn).CloseWrite()
	// expecting `HTTP/1.0 200 OK\r\n...` :
	byteStop := byte('H')
	res, found, err := fnet.SmallReadUntil(d, byteStop, 1) // should read the H and use it as stop byte
	if res == nil || len(res) != 0 || !found || err != nil {
		t.Errorf("Unexpected result %v, %v, %v for fnet.SmallReadUntil() 1 separator", res, found, err)
	}
	byteStop = byte(' ')
	res, found, err = fnet.SmallReadUntil(d, byteStop, 7)
	sres := string(res)
	if sres != "TTP/1.0" || found || err != nil {
		t.Errorf("Unexpected result %q (%v), %v, %v for fnet.SmallReadUntil() 7/exact not found", sres, res, found, err)
	}
	byteStop = byte('2')
	res, found, err = fnet.SmallReadUntil(d, byteStop, 2)
	sres = string(res)
	if sres != " " || !found || err != nil {
		t.Errorf("Unexpected result %q (%v), %v, %v for fnet.SmallReadUntil() 2/exact found", sres, res, found, err)
	}
	byteStop = byte('\r')
	res, found, err = fnet.SmallReadUntil(d, byteStop, 128)
	sres = string(res)
	if sres != "00 OK" || !found || err != nil {
		t.Errorf("Unexpected result %q (%v), %v, %v for fnet.SmallReadUntil() remaining of first line found", sres, res, found, err)
	}
	res, found, err = fnet.SmallReadUntil(d, byteStop, 128)
	sres = string(res)
	// second line (this can break whenever google changes something)
	expected := "\nContent-Type: text/html; charset=ISO-8859-1"
	if sres != expected || !found || err != nil {
		t.Errorf("Unexpected result %q (%v), %v, %v for fnet.SmallReadUntil() second line found", sres, res, found, err)
	}
}

func TestSmallReadUntilTimeOut(t *testing.T) {
	d, err := net.Dial("tcp", "www.google.com:80")
	if err != nil {
		t.Fatalf("can't connect to google to test: %v", err)
	}
	defer d.Close()
	_ = d.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	res, found, err := fnet.SmallReadUntil(d, 0, 200)
	if res == nil || len(res) != 0 || found || !os.IsTimeout(err) {
		t.Errorf("Unexpected result %v, %v, %v for fnet.SmallReadUntil() with timeout", res, found, err)
	}
}

func TestBadGetUniqueUnixDomainPath(t *testing.T) {
	badPath := []byte{0x41, 0, 0x42}
	fname := fnet.GetUniqueUnixDomainPath(string(badPath))
	if fname != "/tmp/fortio-default-uds" {
		t.Errorf("Got %s when expecting default/error case for bad prefix", fname)
	}
}

func TestDefaultGetUniqueUnixDomainPath(t *testing.T) {
	n1 := fnet.GetUniqueUnixDomainPath("")
	n2 := fnet.GetUniqueUnixDomainPath("")
	if n1 == n2 {
		t.Errorf("Got %s and %s when expecting unique names", n1, n2)
	}
}

func TestUnixDomain(t *testing.T) {
	// Test through the proxy as well (which indirectly tests fnet.Listen)
	fname := fnet.GetUniqueUnixDomainPath("fortio-uds-test")
	addr := fnet.ProxyToDestination(fname, "www.google.com:80")
	defer os.Remove(fname) // to not leak the temp socket
	if addr == nil {
		t.Fatalf("Nil socket in unix socket proxy listen")
	}
	hp := fnet.NormalizeHostPort("", addr)
	expected := fmt.Sprintf("-unix-socket=%s", fname)
	if hp != expected {
		t.Errorf("Got %s, expected %s from fnet.NormalizeHostPort(%v)", hp, expected, addr)
	}
	dAddr := net.UnixAddr{Name: fname, Net: fnet.UnixDomainSocket}
	d, err := net.DialUnix(fnet.UnixDomainSocket, nil, &dAddr)
	if err != nil {
		t.Fatalf("can't connect to our proxy using unix socket %v: %v", fname, err)
	}
	defer d.Close()
	data := "HEAD / HTTP/1.0\r\nUser-Agent: fortio-unit-test-" + version.Long() + "\r\n\r\n"
	_, _ = d.Write([]byte(data))
	_ = d.CloseWrite()
	res := make([]byte, 4096)
	n, err := d.Read(res)
	if err != nil {
		t.Errorf("read error with proxy: %v", err)
	}
	resStr := string(res[:n])
	expectedStart := "HTTP/1.0 200 OK\r\n"
	if !strings.HasPrefix(resStr, expectedStart) {
		t.Errorf("Unexpected reply '%q', expected starting with '%q'", resStr, expectedStart)
	}
}

func TestProxyErrors(t *testing.T) {
	addr := fnet.ProxyToDestination(":0", "doesnotexist.fortio.org:80")
	dAddr := net.TCPAddr{Port: addr.(*net.TCPAddr).Port}
	d, err := net.DialTCP("tcp", nil, &dAddr)
	if err != nil {
		t.Fatalf("can't connect to our proxy: %v", err)
	}
	defer d.Close()
	res := make([]byte, 4096)
	n, err := d.Read(res)
	if err == nil {
		t.Errorf("didn't get expected error with proxy %d", n)
	}
	// 2nd proxy on same port should fail
	addr2 := fnet.ProxyToDestination(fnet.GetPort(addr), "www.google.com:80")
	if addr2 != nil {
		t.Errorf("Second proxy on same port should have failed, got %+v", addr2)
	}
}

func TestResolveIpV6(t *testing.T) {
	addr, err := fnet.ResolveByProto("[::1]", "http", "tcp")
	addrStr := addr.String()
	expected := "[::1]:80"
	if addrStr != expected {
		t.Errorf("Got '%s' instead of '%s': %v", addrStr, expected, err)
	}
}

func TestResolveBW(t *testing.T) {
	addr, err := fnet.Resolve("8.8.8.8", "zzzzzz")
	if err == nil {
		t.Errorf("should have errored out but got %v", addr)
	}
	addr, err = fnet.Resolve("8.8.4.4", "domain")
	if err != nil {
		t.Errorf("should have not errored out but got %v", err)
	}
	expecting := "8.8.4.4:53"
	if addr.String() != expecting {
		t.Errorf("expecting %q got %q", expecting, addr.String())
	}
	addr, err = fnet.ResolveDestination("8.8.4.4:domain")
	if err != nil {
		t.Errorf("should hav enot  errored out but got %v", err)
	}
	if addr.String() != expecting {
		t.Errorf("expecting %q got %q", expecting, addr.String())
	}
}

// This test relies on google answer 2 ips, first ipv4, second ipv6.
// if that's not the case anymore or in the testing environment, this will fail.
func TestDNSMethods(t *testing.T) {
	err := fnet.FlagResolveMethod.Set("first")
	if err != nil {
		t.Errorf("unexpected error setting method to first: %v", err)
	}
	fnet.FlagResolveIPType.Set("ip4")
	addr4, err := fnet.Resolve("www.google.com", "80")
	if err != nil {
		t.Errorf("error ip4 resolving google: %v", err)
	}
	fnet.FlagResolveIPType.Set("ip6")
	addr6, err := fnet.Resolve("www.google.com", "80")
	if err != nil {
		t.Errorf("error ip6 resolving google: %v", err)
	}
	if addr4.String() == addr6.String() {
		t.Errorf("ipv4 %v and ipv6 %v shouldn't be same", addr4, addr6)
	}
	fnet.FlagResolveIPType.Set("ip")
	addrFirst, err := fnet.Resolve("www.google.com", "80")
	if err != nil {
		t.Errorf("error ip any resolving google: %v", err)
	}
	if addrFirst.String() != addr4.String() {
		// dns might change when not in cached mode
		log.Warnf("first ip %v not ipv4 %v", addrFirst, addr4)
	}
	addrSecond, err := fnet.Resolve("www.google.com", "80")
	if err != nil {
		t.Errorf("error ip any resolving (2) google: %v", err)
	}
	if addrFirst.String() != addrSecond.String() {
		log.Warnf("first ip %v not == second %v in first mode", addrFirst, addrSecond)
	}
	err = fnet.FlagResolveMethod.Set("cached-rr")
	if err != nil {
		t.Fatalf("error setting back cached-rr mode: %v", err)
	}
	addrThird, err := fnet.Resolve("www.google.com", "80")
	if err != nil {
		t.Errorf("error ip any resolving (3) google: %v", err)
	}
	if addrFirst.String() != addrThird.String() {
		log.Warnf("first cached ip %v not == first %v in cached-rr mode", addrThird, addrFirst)
	}
	addrFourth, err := fnet.Resolve("www.google.com", "80")
	if err != nil {
		t.Errorf("error ip any resolving (4) google: %v", err)
	}
	if addrFourth.String() != addr6.String() {
		log.Warnf("second cached ip %v not == ipv6 %v in cached-rr mode", addrFourth, addr6)
	}
	if addrFourth.String() == addrThird.String() {
		t.Errorf("in cached rr mode, 2nd call %v shouldn't be same as first %v for google", addrFourth, addrThird)
	}
	// back to first (rr) [only if there are only 2 ips]
	addrFifth, err := fnet.Resolve("www.google.com", "80")
	if err != nil {
		t.Errorf("error ip any resolving (5) google: %v", err)
	}
	if addrThird.String() != addrFifth.String() {
		log.Warnf("third cached ip %v not == back to first %v in cached-rr mode (if only 2 ips)", addrFifth, addrThird)
	}
	// clear cache we'll get first again (if we don't get a completely different one that is)
	fnet.ClearResolveCache()
	addrAfterCache, err := fnet.Resolve("www.google.com", "80")
	if err != nil {
		t.Errorf("error ip any resolving (6) google: %v", err)
	}
	if addrAfterCache.String() == addrFourth.String() {
		t.Errorf("cache clear failure, we still got 2nd ip: %v", addrAfterCache)
	}
	if addrAfterCache.String() != addrThird.String() {
		log.Warnf("after cache clear we expect to get first %v, we got %v", addrThird, addrAfterCache)
	}
	// few extra resolve just for coverage
	err = fnet.FlagResolveMethod.Set("rnd")
	if err != nil {
		t.Errorf("unexpected error setting method to rnd: %v", err)
	}
	_, err = fnet.Resolve("www.google.com", "80")
	if err != nil {
		t.Errorf("unexpected error in rnd mode for resolve of google: %v", err)
	}
	err = fnet.FlagResolveMethod.Set("rr")
	if err != nil {
		t.Errorf("unexpected error setting method to rr: %v", err)
	}
	_, err = fnet.Resolve("www.google.com", "80")
	if err != nil {
		t.Errorf("unexpected error in rr mode for resolve of google: %v", err)
	}
	// put it back to default
	err = fnet.FlagResolveMethod.Set("cached-rr")
	if err != nil {
		t.Errorf("unexpected error setting method to cached-rr: %v", err)
	}
	fnet.FlagResolveIPType.Set("ip4")
}

func TestDNSCacheConcurrency(t *testing.T) {
	// Test isn't actually testing unless you use the debugger but coverage shows the extra if
	// does happen.
	fnet.FlagResolveIPType.Set("ip")
	var wg sync.WaitGroup
	n := 20
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			fnet.Resolve("localhost", "80")
			wg.Done()
		}()
	}
	wg.Wait()
	fnet.FlagResolveIPType.Set("ip4")
}

func TestBadValueForDNSMethod(t *testing.T) {
	err := fnet.FlagResolveMethod.Set("foo")
	if err == nil {
		t.Errorf("passing foo to FlagResolveMethod.Set should error out/fail validation")
	}
}

func TestJoinHostAndPort(t *testing.T) {
	tests := []struct {
		inputPort string
		addr      *net.TCPAddr
		expected  string
	}{
		{"8080", &net.TCPAddr{
			IP:   []byte{192, 168, 2, 3},
			Port: 8081,
		}, "localhost:8081"},
		{"192.168.30.14:8081", &net.TCPAddr{
			IP:   []byte{192, 168, 30, 15},
			Port: 8080,
		}, "192.168.30.15:8080"},
		{
			":8080",
			&net.TCPAddr{
				IP:   []byte{0, 0, 0, 1},
				Port: 8080,
			},
			"localhost:8080",
		},
		{
			"",
			&net.TCPAddr{
				IP:   []byte{192, 168, 30, 14},
				Port: 9090,
			}, "localhost:9090",
		},
		{
			"http",
			&net.TCPAddr{
				IP:   []byte{192, 168, 30, 14},
				Port: 9090,
			}, "localhost:9090",
		},
		{
			"192.168.30.14:9090",
			&net.TCPAddr{
				IP:   []byte{192, 168, 30, 14},
				Port: 9090,
			}, "192.168.30.14:9090",
		},
	}
	for _, test := range tests {
		urlHostPort := fnet.NormalizeHostPort(test.inputPort, test.addr)
		if urlHostPort != test.expected {
			t.Errorf("%s is received  but %s was expected", urlHostPort, test.expected)
		}
	}
}

func TestChangeMaxPayloadSize(t *testing.T) {
	tests := []struct {
		input    int
		expected int
	}{
		// negative test cases
		{-1, 0},
		// lesser than current default
		{0, 0},
		{64, 64},
		// Greater than current default
		{987 * 1024, 987 * 1024},
	}
	for _, tst := range tests {
		fnet.ChangeMaxPayloadSize(tst.input)
		actual := len(fnet.Payload)
		if len(fnet.Payload) != tst.expected {
			t.Errorf("Got %d, expected %d for fnet.ChangeMaxPayloadSize(%d)", actual, tst.expected, tst.input)
		}
	}
}

func TestValidatePayloadSize(t *testing.T) {
	fnet.ChangeMaxPayloadSize(256 * 1024)
	tests := []struct {
		input    int
		expected int
	}{
		{257 * 1024, fnet.MaxPayloadSize},
		{10, 10},
		{0, 0},
		{-1, 0},
	}
	for _, test := range tests {
		size := test.input
		fnet.ValidatePayloadSize(&size)
		if size != test.expected {
			t.Errorf("Got %d, expected %d for fnet.ValidatePayloadSize(%d)", size, test.expected, test.input)
		}
	}
}

func TestGenerateRandomPayload(t *testing.T) {
	fnet.ChangeMaxPayloadSize(256 * 1024)
	tests := []struct {
		input    int
		expected int
	}{
		{257 * 1024, fnet.MaxPayloadSize},
		{10, 10},
		{0, 0},
		{-1, 0},
	}
	for _, test := range tests {
		text := fnet.GenerateRandomPayload(test.input)
		if len(text) != test.expected {
			t.Errorf("Got %d, expected %d for GenerateRandomPayload(%d) payload size", len(text), test.expected, test.input)
		}
	}
}

func TestReadFileForPayload(t *testing.T) {
	tests := []struct {
		payloadFile  string
		expectedText []byte
	}{
		{payloadFile: "../.testdata/payloadTest1.txt", expectedText: []byte("{\"test\":\"test\"}")},
		{payloadFile: "", expectedText: nil},
	}

	for _, test := range tests {
		data, err := fnet.ReadFileForPayload(test.payloadFile)
		if err != nil && len(test.expectedText) > 0 {
			t.Errorf("Error should not be happened for ReadFileForPayload")
		}
		if !bytes.Equal(data, test.expectedText) {
			t.Errorf("Got %s, expected %s for ReadFileForPayload()", string(data), string(test.expectedText))
		}
	}
}

func TestGeneratePayload(t *testing.T) {
	tests := []struct {
		payloadFile    string
		payloadSize    int
		payload        string
		expectedResLen int
	}{
		{
			payloadFile: "../.testdata/payloadTest1.txt", payloadSize: 123, payload: "",
			expectedResLen: len("{\"test\":\"test\"}"),
		},
		{
			payloadFile: "nottestmock", payloadSize: 0, payload: "{\"test\":\"test1\"}",
			expectedResLen: 0,
		},
		{
			payloadFile: "", payloadSize: 123, payload: "{\"test\":\"test1\"}",
			expectedResLen: 123,
		},
		{
			payloadFile: "", payloadSize: 0, payload: "{\"test\":\"test1\"}",
			expectedResLen: len("{\"test\":\"test1\"}"),
		},
		{
			payloadFile: "", payloadSize: 0, payload: "",
			expectedResLen: 0,
		},
	}

	for _, test := range tests {
		payload := fnet.GeneratePayload(test.payloadFile, test.payloadSize, test.payload)
		if len(payload) != test.expectedResLen {
			t.Errorf("Got %d, expected %d for GeneratePayload() as payload length", len(payload),
				test.expectedResLen)
		}
	}
}

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
		if actual := fnet.DebugSummary([]byte(tst.input), 8); actual != tst.expected {
			t.Errorf("Got '%s', expected '%s' for DebugSummary(%q)", actual, tst.expected, tst.input)
		}
	}
}

// --- max logging for tests

func init() {
	log.SetLogLevel(log.Debug)
}
