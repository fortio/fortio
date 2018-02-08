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
	"net"
	"strings"
)

// NormalizePort parses port and returns host:port if port is in the form
// of host:port or :port if port is in the form of port.
func NormalizePort(port string) string {
	if strings.ContainsAny(port, ":") {
		return port
	}
	return ":" + port
}

// GRPCDestination parses dest and returns dest:port based on dest type
// being a hostname, IPv4 or IPv6 address.
func GRPCDestination(dest, port string) string {
	if ip := net.ParseIP(dest); ip != nil {
		switch {
		case ip.To4() != nil:
			return ip.String() + NormalizePort(port)
		case ip.To16() != nil:
			return "[" + ip.String() + "]" + NormalizePort(port)
		}

	}
	// dest must be in the form of hostname
	return dest + NormalizePort(port)
}
