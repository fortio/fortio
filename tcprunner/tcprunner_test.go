// Copyright 2020-2021 Fortio Authors.
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
//

package tcprunner

import (
	"fmt"
	"net"
	"runtime"
	"testing"

	"fortio.org/fortio/fnet"
	"fortio.org/fortio/log"
)

func TestTCPRunnerBadDestination(t *testing.T) {
	destination := "doesnotexist.fortio.org:1111"
	opts := RunnerOptions{}
	opts.QPS = 100
	opts.Destination = destination
	res, err := RunTCPTest(&opts)
	if err == nil {
		t.Fatalf("unexpected success on bad destination %+v", res)
	}
	t.Logf("Got expected error: %v", err)
}

func TestTCPRunner(t *testing.T) {
	addr := fnet.TCPEchoServer("test-echo-runner", ":0")
	destination := fmt.Sprintf("tcp://localhost:%d/", addr.(*net.TCPAddr).Port)

	opts := RunnerOptions{}
	opts.QPS = 100
	opts.Destination = destination
	res, err := RunTCPTest(&opts)
	if err != nil {
		t.Error(err)
		return
	}
	totalReq := res.DurationHistogram.Count
	tcpOk := res.RetCodes[TCPStatusOK]
	if totalReq != tcpOk {
		t.Errorf("Mismatch between requests %d and ok %v", totalReq, res.RetCodes)
	}
	if res.SocketCount != res.RunnerResults.NumThreads {
		t.Errorf("%d socket used, expected same as thread# %d", res.SocketCount, res.RunnerResults.NumThreads)
	}
	if res.BytesReceived != res.BytesSent {
		t.Errorf("Bytes received %d should bytes sent %d", res.BytesReceived, res.BytesSent)
	}
}

func TestTCPRunnerLargePayload(t *testing.T) {
	addr := fnet.TCPEchoServer("test-echo-runner", ":0")
	destination := fmt.Sprintf("tcp://localhost:%d/", addr.(*net.TCPAddr).Port)

	opts := RunnerOptions{}
	opts.QPS = 10
	opts.Destination = destination
	opts.Payload = fnet.GenerateRandomPayload(120000)
	log.SetLogLevel(log.Debug)
	res, err := RunTCPTest(&opts)
	if err != nil {
		t.Error(err)
		return
	}
	totalReq := res.DurationHistogram.Count
	tcpOk := res.RetCodes[TCPStatusOK]
	if totalReq != tcpOk {
		t.Errorf("Mismatch between requests %d and ok %v", totalReq, res.RetCodes)
	}
	if res.SocketCount != res.RunnerResults.NumThreads {
		t.Errorf("%d socket used, expected same as thread# %d", res.SocketCount, res.RunnerResults.NumThreads)
	}
	if res.BytesReceived != res.BytesSent {
		t.Errorf("Bytes received %d should bytes sent %d", res.BytesReceived, res.BytesSent)
	}
}

func TestTCPNotLeaking(t *testing.T) {
	opts := &RunnerOptions{}
	ngBefore1 := runtime.NumGoroutine()
	t.Logf("Number go routine before test %d", ngBefore1)
	addr := fnet.TCPEchoServer("test-echo-runner", ":0")
	numCalls := 100
	opts.NumThreads = numCalls / 2 // make 2 calls per thread
	opts.Exactly = int64(numCalls)
	opts.QPS = float64(numCalls) / 2 // take 1 second
	opts.Destination = fmt.Sprintf("localhost:%d", addr.(*net.TCPAddr).Port)
	// Warm up round 1
	_, err := RunTCPTest(opts)
	if err != nil {
		t.Error(err)
		return
	}
	ngBefore2 := runtime.NumGoroutine()
	t.Logf("Number of go routine after warm up / before 2nd test %d", ngBefore2)
	// 2nd run, should be stable number of go routines after first, not keep growing:
	res, err := RunTCPTest(opts)
	// it takes a while for the connections to close with std client (!) why isn't CloseIdleConnections() synchronous
	runtime.GC()
	runtime.GC() // 2x to clean up more... (#178)
	ngAfter := runtime.NumGoroutine()
	t.Logf("Number of go routine after 2nd test %d", ngAfter)
	if err != nil {
		t.Error(err)
		return
	}
	// allow for ~8 goroutine variance, as we use 50 if we leak it will show (was failing before #167)
	if ngAfter > ngBefore2+8 {
		t.Errorf("Goroutines after test %d, expected it to stay near %d", ngAfter, ngBefore2)
	}
	if res.SocketCount != res.RunnerResults.NumThreads {
		t.Errorf("%d socket used, expected same as thread# %d", res.SocketCount, res.RunnerResults.NumThreads)
	}
}
