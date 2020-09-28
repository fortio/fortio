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
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
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
		// Good cases:
		{"using ip:portname", "8.8.8.8:http", "8.8.8.8:80"},
		{"using ip:port", "8.8.8.8:12345", "8.8.8.8:12345"},
	}
	for _, tt := range tests {
		tt := tt // pin
		t.Run(tt.name, func(t *testing.T) {
			got := fnet.ResolveDestination(tt.destination)
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
	addr := fnet.ResolveDestination("www.google.com:443")
	t.Logf("Found google addr %+v", addr)
	if addr == nil {
		t.Error("got nil address for google")
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

func TestTCPEchoServerErrors(t *testing.T) {
	addr := fnet.TCPEchoServer("test-tcp-echo", ":0")
	dAddr := net.TCPAddr{Port: addr.(*net.TCPAddr).Port}
	port := dAddr.String()[1:]
	log.Infof("Connecting to %q", port)
	log.SetLogLevel(log.Debug)
	// 2nd proxy on same port should fail
	addr2 := fnet.TCPEchoServer("test-tcp-echo-error", fnet.GetPort(addr))
	if addr2 != nil {
		t.Errorf("Second proxy on same port should have failed, got %+v", addr2)
	}
	// For some reason unable to trigger these 2 cases within go
	// TODO: figure it out... this is now only triggering coverage but not really testing anything
	err1 := "cat /dev/zero | nc -v localhost " + port + " 1>&-" // write error
	cmd := exec.Command("/bin/bash", "-c", err1)
	err := cmd.Run()
	log.Infof("cmd1 %q ran, error: %v", err1, err)
	err2 := "cat /dev/zero | nc -v localhost " + port + " | (sleep 1; echo end)" // read error
	cmd = exec.Command("/bin/bash", "-c", err2)
	err = cmd.Run()
	log.Infof("cmd2 %q ran, error: %v", err2, err)
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
	addr := fnet.Resolve("[::1]", "http")
	addrStr := addr.String()
	expected := "[::1]:80"
	if addrStr != expected {
		t.Errorf("Got '%s' instead of '%s'", addrStr, expected)
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

// --- max logging for tests

func init() {
	log.SetLogLevel(log.Debug)
}
