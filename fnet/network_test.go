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

package fnet

import (
	"net"
	"strconv"
	"strings"
	"testing"

	"istio.io/fortio/log"
	"istio.io/fortio/version"
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
		port := NormalizePort(tc.input)
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
	l, a := Listen("test listen1", "0")
	if l == nil || a == nil {
		t.Fatalf("Unexpected nil in Listen() %v %v", l, a)
	}
	if a.Port == 0 {
		t.Errorf("Unexpected 0 port after listen %+v", a)
	}
	_ = l.Close() // nolint: gas
}

func TestListenFailure(t *testing.T) {
	_, a1 := Listen("test listen2", "0")
	if a1.Port == 0 {
		t.Errorf("Unexpected 0 port after listen %+v", a1)
	}
	l, a := Listen("this should fail", strconv.Itoa(a1.Port))
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
		{"using bogus hostname", "doesnotexist.istio.io:443", ""},
		// Good cases:
		{"using ip:portname", "8.8.8.8:http", "8.8.8.8:80"},
		{"using ip:port", "8.8.8.8:12345", "8.8.8.8:12345"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveDestination(tt.destination)
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

func TestSetGRPCDestination(t *testing.T) {
	tests := []struct {
		name   string
		dest   string
		output string
	}{
		{
			"valid hostname",
			"localhost",
			"localhost:8079",
		},
		{
			"invalid hostname",
			"lclhst",
			"lclhst",
		},
		{
			"valid hostname and port",
			"localhost:1234",
			"localhost:1234",
		},
		{
			"invalid hostname with port",
			"lclhst:1234",
			"lclhst:1234",
		},
		{
			"valid hostname with http prefix",
			"http://localhost",
			"localhost:80",
		},
		{
			"invalid hostname with http prefix",
			"http://lclhst",
			"http://lclhst",
		},
		{
			"valid hostname with https prefix",
			"https://localhost",
			"localhost:443",
		},
		{
			"invalid hostname with https prefix",
			"https://loclhst",
			"https://loclhst",
		},
		{
			"valid IPv4 address",
			"1.2.3.4",
			"1.2.3.4:8079",
		},
		{
			"invalid IPv4 address",
			"1.2.3..4",
			"1.2.3..4",
		},
		{
			"valid IPv4 address and port",
			"1.2.3.4:5678",
			"1.2.3.4:5678",
		},
		{
			"invalid IPv4 address with port",
			"1.2.3..4:1234",
			"1.2.3..4:1234",
		},
		{
			"valid IPv6 address",
			"2001:dba::1",
			"[2001:dba::1]:8079",
		},
		{
			"invalid IPv6 address",
			"2001:dba:::1",
			"2001:dba:::1",
		},
		{
			"valid IPv6 address and port",
			"[2001:dba::1]:1234",
			"[2001:dba::1]:1234",
		},
		{
			"invalid IPv6 address and port",
			"[2001:dba:::1]:1234",
			"[2001:dba:::1]:1234",
		},
		{
			"valid IPv6 address with http prefix",
			"http://2001:dba::1",
			"[2001:dba::1]:80",
		},
		{
			"invalid IPv6 address with http prefix",
			"http://2001:dba:::1",
			"http://2001:dba:::1",
		},
		{
			"valid IPv6 address and port with https prefix",
			"https://2001:dba::1",
			"[2001:dba::1]:443",
		},
		{
			"invalid IPv6 address and port with https prefix",
			"https://2001:dba:::1",
			"https://2001:dba:::1",
		},
	}

	for _, tc := range tests {
		dest := SetGRPCDestination(tc.dest)
		if dest != tc.output {
			t.Errorf("Test case: %s failed to set gRPC destination\n\texpected: %s\n\t  actual: %s",
				tc.name,
				tc.output,
				dest,
			)
		}
	}
}

func TestResolveDestinationMultipleIps(t *testing.T) {
	addr := ResolveDestination("www.google.com:443")
	t.Logf("Found google addr %+v", addr)
	if addr == nil {
		t.Error("got nil address for google")
	}
}

func TestProxy(t *testing.T) {
	addr := ProxyToDestination(":0", "www.google.com:80")
	dAddr := net.TCPAddr{Port: addr.Port}
	d, err := net.DialTCP("tcp", nil, &dAddr)
	if err != nil {
		t.Fatalf("can't connect to our proxy: %v", err)
	}
	defer d.Close()
	data := "HEAD / HTTP/1.0\r\nUser-Agent: fortio-unit-test-" + version.Long() + "\r\n\r\n"
	d.Write([]byte(data))
	d.CloseWrite()
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
	addr := ProxyToDestination(":0", "doesnotexist.istio.io:80")
	dAddr := net.TCPAddr{Port: addr.Port}
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
	addr2 := ProxyToDestination(strconv.Itoa(addr.Port), "www.google.com:80")
	if addr2 != nil {
		t.Errorf("Second proxy on same port should have failed, got %+v", addr2)
	}
}
func TestResolveIpV6(t *testing.T) {
	addr := Resolve("[::1]", "http")
	addrStr := addr.String()
	expected := "[::1]:80"
	if addrStr != expected {
		t.Errorf("Got '%s' instead of '%s'", addrStr, expected)
	}
}

// --- max logging for tests

func init() {
	log.SetLogLevel(log.Debug)
}
