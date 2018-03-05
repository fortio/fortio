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
	"net"
	"strings"

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

// Listen returns a listener for the port. Port can be a port or a
// bind address and a port (e.g. "8080" or "[::1]:8080"...). If the
// port component is 0 a free port will be returned by the system.
// This logs fatal on error and is meant for servers that must start.
// For library use, we could extract into 2 functions, one returning
// error,... if needed.
func Listen(name string, port string) (net.Listener, *net.TCPAddr) {
	nPort := NormalizePort(port)
	listener, err := net.Listen("tcp", nPort)
	if err != nil {
		log.Fatalf("Can't listen to %v: %v", nPort, err)
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
	i := strings.LastIndex(dest, ":")
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
	addrs, err := net.LookupIP(host)
	if err != nil {
		log.Errf("Unable to lookup '%s' : %v", host, err)
		return nil
	}
	if len(addrs) > 1 && log.LogDebug() {
		log.Debugf("Using only the first of the addresses for %s : %v", host, addrs)
	}
	log.Debugf("Will go to %s", addrs[0])
	dest := &net.TCPAddr{}
	dest.IP = addrs[0]
	dest.Port, err = net.LookupPort("tcp", port)
	if err != nil {
		log.Errf("Unable to resolve port '%s' : %v", port, err)
		return nil
	}
	return dest
}

/*
// Proxy starts a tcp proxy.
func Proxy(port string, dest *net.TCPAddr) {
	l, a := Listen("proxy", port)
	d, err := net.DialTCP("tcp", nil, dest)
	if err != nil {
		log.Errf("Unable to connect to %v : %v", dest, err)
		return nil
	}

}
*/
