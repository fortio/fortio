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
// of host:port or :port if port is in the form of port.
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

/*
// Proxy starts a tcp proxy.
func Proxy(port string) {
}
*/
