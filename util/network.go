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

package util // import "istio.io/fortio/util"

import (
	"fmt"
	"net"
	"strconv"
)

const (
	// MinTCPPort is the minimum TCP port number that can be used by Fortio http server.
	MinTCPPort int = 1024
	// MaxTCPPort is the maximum TCP port number that can be used by Fortio http server.
	MaxTCPPort int = 65535
)

// NormalizePort validates hostport and returns hostport as :port or host:port.
// If the host:port or port are invalid, NormailzePort returns an error.
func NormalizePort(hostport string) (string, error) {
	host, port, err := net.SplitHostPort(hostport)
	switch {
	case err != nil:
		// hostport must not be in the form of host:port. Check to see if hostport is a valid TCP port number.
		if !ValidatePort(hostport) {
			return "", fmt.Errorf("Port %s is outside of supported range %d-%d", port, MinTCPPort, MaxTCPPort)
		}
		return "0.0.0.0:" + hostport, nil
	case err == nil:
		// Make sure host:port is a proper IP and port is within unreserved range.
		if !ValidateIP(host) {
			return "", fmt.Errorf("Error validating IP: %s", host)
		}
		if !ValidatePort(port) {
			return "", fmt.Errorf("Port %s is outside of supported range %d-%d", port, MinTCPPort, MaxTCPPort)
		}
		return net.JoinHostPort(host, port), nil
	default:
		return "", fmt.Errorf("Unable to normalize hostport: %s", hostport)
	}
}

// ValidateIP checks that ip is a valid IP address or hostname.
func ValidateIP(ip string) bool {
	// Use local resolver in case ip represents a hostname such as localhost.
	if _, err := net.LookupIP(ip); err == nil {
		return true
	}
	if ipAddr := net.ParseIP(ip); ipAddr != nil {
		return true
	}
	return false
}

// ValidatePort checks that port is a valid port from the unreserved TCP range.
func ValidatePort(port string) bool {
	pInt, err := strconv.Atoi(port)
	if err != nil {
		return false
	}
	if pInt > MinTCPPort && pInt < MaxTCPPort {
		return true
	}
	return false
}
