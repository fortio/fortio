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

package fnet // import "fortio.org/fortio/fnet"

import (
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"

	"fortio.org/fortio/log"
	"fortio.org/fortio/version"
)

const (
	// DefaultGRPCPort is the Fortio gRPC server default port number.
	DefaultGRPCPort = "8079"
	// StandardHTTPPort is the Standard http port number.
	StandardHTTPPort = "80"
	// StandardHTTPSPort is the Standard https port number.
	StandardHTTPSPort = "443"
	// PrefixHTTP is a constant value for representing http protocol that can be added prefix of url.
	PrefixHTTP = "http://"
	// PrefixHTTPS is a constant value for representing secure http protocol that can be added prefix of url.
	PrefixHTTPS = "https://"

	// POST is a constant value that indicates http method as post.
	POST = "POST"
	// GET is a constant value that indicates http method as get.
	GET = "GET"
	// UnixDomainSocket type for network addresses.
	UnixDomainSocket = "unix"
)

var (
	// KILOBYTE is a constant for kilobyte (ie 1024).
	KILOBYTE = 1024
	// MaxPayloadSize is the maximum size of payload to be generated by the
	// EchoHandler size= argument. In bytes.
	MaxPayloadSize = 256 * KILOBYTE
	// Payload that is returned during echo call.
	Payload []byte
)

// nolint: gochecknoinits // needed here (unit change)
func init() {
	ChangeMaxPayloadSize(MaxPayloadSize)
}

// ChangeMaxPayloadSize is used to change max payload size and fill it with pseudorandom content.
func ChangeMaxPayloadSize(newMaxPayloadSize int) {
	if newMaxPayloadSize >= 0 {
		MaxPayloadSize = newMaxPayloadSize
	} else {
		MaxPayloadSize = 0
	}
	Payload = make([]byte, MaxPayloadSize)
	// One shared and 'constant' (over time) but pseudo random content for payload
	// (to defeat compression).
	_, err := rand.Read(Payload) // nolint: gosec // We don't need crypto strength here, just low cpu and speed
	if err != nil {
		log.Errf("Error changing payload size, read for %d random payload failed: %v", newMaxPayloadSize, err)
	}
}

// NormalizePort parses port and returns host:port if port is in the form
// of host:port already or :port if port is only a port (doesn't contain :).
func NormalizePort(port string) string {
	if strings.ContainsAny(port, ":") {
		return port
	}
	return ":" + port
}

// Listen returns a listener for the port. Port can be a port or a
// bind address and a port (e.g. "8080" or "[::1]:8080"...). If the
// port component is 0 a free port will be returned by the system.
// If the port is a pathname (contains a /) a unix domain socket listener
// will be used instead of regular tcp socket.
// This logs critical on error and returns nil (is meant for servers
// that must start).
func Listen(name string, port string) (net.Listener, net.Addr) {
	sockType := "tcp"
	nPort := port
	if strings.Contains(port, "/") {
		sockType = UnixDomainSocket
	} else {
		nPort = NormalizePort(port)
	}
	listener, err := net.Listen(sockType, nPort)
	if err != nil {
		log.Critf("Can't listen to %s socket %v (%v) for %s: %v", sockType, port, nPort, name, err)
		return nil, nil
	}
	lAddr := listener.Addr()
	if len(name) > 0 {
		fmt.Printf("Fortio %s %s server listening on %s\n", version.Short(), name, lAddr)
	}
	return listener, lAddr
}

// GetPort extracts the port for TCP sockets and the path for unix domain sockets.
func GetPort(lAddr net.Addr) string {
	var lPort string
	// Note: might panic if called with something else than unix or tcp socket addr, it's ok.
	if lAddr.Network() == UnixDomainSocket {
		lPort = lAddr.(*net.UnixAddr).Name
	} else {
		lPort = strconv.Itoa(lAddr.(*net.TCPAddr).Port)
	}
	return lPort
}

// ResolveDestination returns the TCP address of the "host:port" suitable for net.Dial.
// nil in case of errors.
func ResolveDestination(dest string) net.Addr {
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
func Resolve(host string, port string) net.Addr {
	log.Debugf("Resolve() called with host=%s port=%s", host, port)
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

func transfer(wg *sync.WaitGroup, dst net.Conn, src net.Conn) {
	n, oErr := io.Copy(dst, src) // keep original error for logs below
	log.LogVf("Proxy: transferred %d bytes from %v to %v (err=%v)", n, src.RemoteAddr(), dst.RemoteAddr(), oErr)
	sTCP, ok := src.(*net.TCPConn)
	if ok {
		err := sTCP.CloseRead()
		if err != nil { // We got an eof so it's already half closed.
			log.LogVf("Proxy: semi expected error CloseRead on src %v: %v,%v", src.RemoteAddr(), err, oErr)
		}
	}
	dTCP, ok := dst.(*net.TCPConn)
	if ok {
		err := dTCP.CloseWrite()
		if err != nil {
			log.Errf("Proxy: error CloseWrite on dst %v: %v,%v", dst.RemoteAddr(), err, oErr)
		}
	}
	wg.Done()
}

// ErrNilDestination returned when trying to proxy to a nil address.
var ErrNilDestination = fmt.Errorf("nil destination")

func handleProxyRequest(conn net.Conn, dest net.Addr) {
	err := ErrNilDestination
	var d net.Conn
	if dest != nil {
		d, err = net.Dial(dest.Network(), dest.String())
	}
	if err != nil {
		log.Errf("Proxy: unable to connect to %v for %v : %v", dest, conn.RemoteAddr(), err)
		_ = conn.Close()
		return
	}
	var wg sync.WaitGroup
	wg.Add(2) // 2 threads to wait for...
	go transfer(&wg, d, conn)
	transfer(&wg, conn, d)
	wg.Wait()
	log.LogVf("Proxy: both sides of transfer to %v for %v done", dest, conn.RemoteAddr())
	// Not checking as we are closing/ending anyway - note: bad side effect of coverage...
	_ = d.Close()
	_ = conn.Close()
}

// Proxy starts a tcp proxy.
func Proxy(port string, dest net.Addr) net.Addr {
	listener, lAddr := Listen(fmt.Sprintf("proxy for %v", dest), port)
	if listener == nil {
		return nil // error already logged
	}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				log.Critf("Proxy: error accepting: %v", err) // will this loop with error?
			} else {
				log.LogVf("Proxy: Accepted proxy connection from %v -> %v (for listener %v)",
					conn.RemoteAddr(), conn.LocalAddr(), dest)
				// TODO limit number of go request, use worker pool, etc...
				go handleProxyRequest(conn, dest)
			}
		}
	}()
	return lAddr
}

// ProxyToDestination opens a proxy from the listenPort (or addr:port or unix domain socket path) and forwards
// all traffic to destination (host:port).
func ProxyToDestination(listenPort string, destination string) net.Addr {
	return Proxy(listenPort, ResolveDestination(destination))
}

// NormalizeHostPort generates host:port string for the address or uses localhost instead of [::]
// when the original port binding input didn't specify an address.
func NormalizeHostPort(inputPort string, addr net.Addr) string {
	urlHostPort := addr.String()
	if addr.Network() == UnixDomainSocket {
		urlHostPort = fmt.Sprintf("-unix-socket=%s", urlHostPort)
	} else {
		if strings.HasPrefix(inputPort, ":") || !strings.Contains(inputPort, ":") {
			urlHostPort = fmt.Sprintf("localhost:%d", addr.(*net.TCPAddr).Port)
		}
	}
	return urlHostPort
}

// ValidatePayloadSize compares input size with MaxPayLoadSize. If size exceeds the MaxPayloadSize
// size will set to MaxPayLoadSize.
func ValidatePayloadSize(size *int) {
	if *size > MaxPayloadSize && *size > 0 {
		log.Warnf("Requested size %d greater than max size %d, using max instead (change max using -maxpayloadsizekb)",
			*size, MaxPayloadSize)
		*size = MaxPayloadSize
	} else if *size < 0 {
		log.Warnf("Requested size %d is negative, using 0 (no additional payload) instead.", *size)
		*size = 0
	}
}

// GenerateRandomPayload generates a random payload with given input size.
func GenerateRandomPayload(payloadSize int) []byte {
	ValidatePayloadSize(&payloadSize)
	return Payload[:payloadSize]
}

// ReadFileForPayload reads the file from given input path.
func ReadFileForPayload(payloadFilePath string) ([]byte, error) {
	data, err := ioutil.ReadFile(payloadFilePath)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// GeneratePayload generates a payload with given inputs.
// First tries filePath, then random payload, at last payload.
func GeneratePayload(payloadFilePath string, payloadSize int, payload string) []byte {
	if len(payloadFilePath) > 0 {
		p, err := ReadFileForPayload(payloadFilePath)
		if err != nil {
			log.Warnf("File read operation is failed %v", err)
			return nil
		}
		return p
	} else if payloadSize > 0 {
		return GenerateRandomPayload(payloadSize)
	} else {
		return []byte(payload)
	}
}

// GetUniqueUnixDomainPath returns a path to be used for unix domain socket.
func GetUniqueUnixDomainPath(prefix string) string {
	if prefix == "" {
		prefix = "fortio-uds"
	}
	f, err := ioutil.TempFile(os.TempDir(), prefix)
	if err != nil {
		log.Errf("Unable to generate temp file with prefix %s: %v", prefix, err)
		return "/tmp/fortio-default-uds"
	}
	fname := f.Name()
	_ = f.Close()
	// for the bind to succeed we need the file to not pre exist:
	_ = os.Remove(fname)
	return fname
}

// SmallReadUntil will read one byte at a time until stopByte is found and up to max bytes total.
// Returns what was read (without the stop byte when found), whether the stop byte was found, whether an error occurred (eof...).
// Because we read one by one directly (no buffer) this should only be used for short variable length preamble type read.
func SmallReadUntil(r io.Reader, stopByte byte, max int) ([]byte, bool, error) {
	buf := make([]byte, max)
	i := 0
	for i < max {
		n, err := r.Read(buf[i : i+1])
		if err != nil {
			return buf[0:i], false, err
		}
		if n != 1 {
			log.Critf("Bug/unexpected case, read %d instead of 1 byte yet no error", n)
		}
		if buf[i] == stopByte {
			return buf[0:i], true, nil
		}
		i += n
	}
	return buf[0:i], false, nil
}
