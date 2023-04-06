// Copyright 2017-2023 Fortio Authors
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

//go:build !race

package fnet_test

import (
	"context"
	"io"
	"net"
	"strings"
	"testing"

	"fortio.org/fortio/fnet"
	"fortio.org/log"
)

func TestTCPEchoServerEOF(t *testing.T) {
	addr := fnet.TCPEchoServer("test-tcp-echo", ":0")
	dAddr := net.TCPAddr{Port: addr.(*net.TCPAddr).Port}
	port := dAddr.String()
	log.Infof("Connecting to %q", port)
	log.SetLogLevel(log.Verbose)
	// For some reason unable to trigger these 2 cases within go
	// TODO: figure it out... this is now only triggering coverage but not really testing anything
	// quite brittle but somehow we can get read: connection reset by peer and write: broken pipe
	// with these timings (!)
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		eofStopFlag := (i%2 == 0)
		in := io.NopCloser(strings.NewReader(strings.Repeat("x", 50000)))
		var out ErroringWriter
		err := fnet.NetCat(ctx, "localhost"+port, in, &out, eofStopFlag)
		if err == nil {
			t.Errorf("NetCat expected to get error %v %v", eofStopFlag, err)
		}
	}
}
