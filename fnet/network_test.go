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
	"strconv"
	"testing"
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
	l, a := Listen("test listen", "0")
	if l == nil || a == nil {
		t.Fatalf("Unexpected nil in Listen() %v %v", l, a)
	}
	if a.Port == 0 {
		t.Errorf("Unexpected 0 port after listen %+v", a)
	}
	_ = l.Close() // nolint: gas
}

func TestListenFailure(t *testing.T) {
	reached := false
	defer func(rp *bool) {
		if r := recover(); r == nil {
			t.Error("expected a panic from listen, didn't get one")
		}
		if !*rp {
			t.Error("didn't reach expected statement")
		}
	}(&reached)
	_, a1 := Listen("test listen1", "0")
	if a1.Port == 0 {
		t.Errorf("Unexpected 0 port after listen %+v", a1)
	}
	reached = true // last reached statement
	_, _ = Listen("this should fail", strconv.Itoa(a1.Port))
	t.Error("should not reach this")
}
