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

package fnet // import "istio.io/fortio/fnet"

import (
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"sync"

	"istio.io/fortio/log"
	"istio.io/fortio/version"
)

// NormalizePort parses port and returns host:port if port is in the form
// of host:port already or :port if port is only a port (doesn't contain :).
func NormalizePort(port string) string {
	if strings.ContainsAny(port, ":") {
		return port
	}
	return ":" + port
}

// BracketizeIPv6Address returns s in brackets if s is an IPv6 address.
func BracketizeIPv6Address(s string) string {
	ip := net.ParseIP(s)
	switch {
	case ip == nil:
		// s is not a valid ip.
		log.Errf("Invalid IP address: %s", s)
		return s
	case ip.To4() != nil:
		// s is an IPv4 address, do not wrap s in brackets.
		log.Errf("Address is IPv4, not wrapping %s in brackets.", s)
		return s
	default:
		// s must be an IPv6 address, so wrap s in brackets.
		log.Infof("Address is IPv6, wrapping %s in brackets.", s)
		return "[" + s + "]"
	}
}

// AppendPort parses s as a url and returns s with the host-portion of s as s:port.
// s is returned unmodified if s cannot be parsed or the host-portion of s is an
// invalid domain name or invalid IP address.
func AppendPort(s string) string {
	u, err := url.Parse(s)
	if err != nil {
		log.Errf("Unable to Parse URL %s: %v", err)
		return s
	}
	var port string
	switch {
	case u.Scheme == "http":
		port = "80"
	case u.Scheme == "https":
		port = "443"
	default:
		// Unsupported scheme for valid url s
		log.Errf("Unsupported URL scheme: %s", u.Scheme)
		return s
	}
	ip := net.ParseIP(u.Host)
	if ip != nil {
		switch {
		case ip.To4() != nil:
			// The host portion of s is an IPv4 address,
			// append port.
			log.Infof("Appending %s with port %s", u.Host, port)
			u.Host += NormalizePort(port)
		case ip.To16() != nil:
			// The host portion of s is an IPv6 address without brackets,
			// bracketize and append port.
			log.Infof("Appending %s with port %s", u.Host, NormalizePort(port))
			u.Host = BracketizeIPv6Address(u.Host) + NormalizePort(port)
		}
	} else {
		// Check if the host portion of s is an IPv6 address
		// wrapped in brackets (i.e. rfc 2732)
		if strings.HasPrefix(u.Host, "[") && strings.HasSuffix(u.Host, "]") {
			trimHost := strings.TrimSuffix(strings.TrimPrefix(u.Host, "["), "]")
			ip := net.ParseIP(trimHost)
			if ip != nil {
				// The host portion of s is valid IPv6 address
				// wrapped in brackets
				log.Infof("Appending %s with port %s", u.Host, port)
				u.Host += NormalizePort(port)
				return u.String()
			}
			// The bracketed IPv6 address is invalid,
			// return s unmodified.
			log.Errf("Invalid IPv6 address wrapped in brackets: %v", u.Host)
			return s
		}
		// Check if the host portion of s is a valid domain name.
		_, err := net.LookupHost(u.Host)
		// The host portion of s is an invalid domain name or invalid IP address,
		// return s unmodified.
		if err != nil {
			log.Errf("Invalid domain name or IP address in URL: %s", u.String())
			return s
		}
		log.Infof("Appending %s with port %s", u.Host, NormalizePort(port))
		u.Host += NormalizePort(port)
	}
	return u.String()
}

// Listen returns a listener for the port. Port can be a port or a
// bind address and a port (e.g. "8080" or "[::1]:8080"...). If the
// port component is 0 a free port will be returned by the system.
// This logs critical on error and returns nil (is meant for servers
// that must start).
func Listen(name string, port string) (net.Listener, *net.TCPAddr) {
	nPort := NormalizePort(port)
	listener, err := net.Listen("tcp", nPort)
	if err != nil {
		log.Critf("Can't listen to %v: %v", nPort, err)
		return nil, nil
	}
	addr := listener.Addr().(*net.TCPAddr)
	if len(name) > 0 {
		fmt.Printf("Fortio %s %s server listening on %s\n", version.Short(), name, addr.String())
	}
	return listener, addr
}

// ResolveDestination returns the TCP address of the "host:port" suitable for net.Dial.
// nil in case of errors.
func ResolveDestination(dest string) *net.TCPAddr {
	i := strings.LastIndex(dest, ":") // important so [::1]:port works
	if i < 0 {
		log.Errf("Destination '%s' is not host:port format", dest)
		return nil
	}
	host := dest[0:i]
	port := dest[i+1:]
	return Resolve(host, port)
}

// Resolve returns the TCP address of the host,port suitable for net.Dial.
// nil in case of errors.
func Resolve(host string, port string) *net.TCPAddr {
	dest := &net.TCPAddr{}
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		log.Debugf("host %s looks like an IPv6, stripping []", host)
		host = host[1 : len(host)-1]
	}
	isAddr := net.ParseIP(host)
	var err error
	if isAddr != nil {
		log.Debugf("Host already an IP, will go to %s", isAddr)
		dest.IP = isAddr
	} else {
		var addrs []net.IP
		addrs, err = net.LookupIP(host)
		if err != nil {
			log.Errf("Unable to lookup '%s' : %v", host, err)
			return nil
		}
		if len(addrs) > 1 && log.LogDebug() {
			log.Debugf("Using only the first of the addresses for %s : %v", host, addrs)
		}
		log.Debugf("Will go to %s", addrs[0])
		dest.IP = addrs[0]
	}
	dest.Port, err = net.LookupPort("tcp", port)
	if err != nil {
		log.Errf("Unable to resolve port '%s' : %v", port, err)
		return nil
	}
	return dest
}

func transfer(wg *sync.WaitGroup, dst *net.TCPConn, src *net.TCPConn) {
	n, oErr := io.Copy(dst, src) // keep original error for logs below
	log.LogVf("Proxy: transferred %d bytes from %v to %v (err=%v)", n, src.RemoteAddr(), dst.RemoteAddr(), oErr)
	err := src.CloseRead()
	if err != nil { // We got an eof so it's already half closed.
		log.LogVf("Proxy: semi expected error CloseRead on src %v: %v,%v", src.RemoteAddr(), err, oErr)
	}
	err = dst.CloseWrite()
	if err != nil {
		log.Errf("Proxy: error CloseWrite on dst %v: %v,%v", dst.RemoteAddr(), err, oErr)
	}
	wg.Done()
}

func handleProxyRequest(conn *net.TCPConn, dest *net.TCPAddr) {
	d, err := net.DialTCP("tcp", nil, dest)
	if err != nil {
		log.Errf("Proxy: unable to connect to %v for %v : %v", dest, conn.RemoteAddr(), err)
		_ = conn.Close()
		return
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go transfer(&wg, d, conn)
	transfer(&wg, conn, d)
	wg.Wait()
	log.LogVf("Proxy: both sides of transfer to %v for %v done", dest, conn.RemoteAddr())
	// Not checking as we are closing/ending anyway - note: bad side effect of coverage...
	_ = d.Close()
	_ = conn.Close()
}

// Proxy starts a tcp proxy.
func Proxy(port string, dest *net.TCPAddr) *net.TCPAddr {
	listener, addr := Listen("proxy for "+dest.String(), port)
	if addr == nil {
		return nil // error already logged
	}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				log.Critf("Proxy: error accepting: %v", err) // will this loop with error?
			} else {
				tcpConn := conn.(*net.TCPConn)
				log.LogVf("Proxy: Accepted proxy connection from %v for %v", conn.RemoteAddr(), dest)
				// TODO limit number of go request, use worker pool, etc...
				go handleProxyRequest(tcpConn, dest)
			}
		}
	}()
	return addr
}

// ProxyToDestination opens a proxy from the listenPort (or addr:port) and forwards
// all traffic to destination (host:port)
func ProxyToDestination(listenPort string, destination string) *net.TCPAddr {
	return Proxy(listenPort, ResolveDestination(destination))
}
