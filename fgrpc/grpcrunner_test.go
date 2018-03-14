// Copyright 2017 Istio Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

// Adapted from istio/proxy/test/backend/echo with error handling and
// concurrency fixes and making it as low overhead as possible
// (no std output by default)

package fgrpc

import (
	"fmt"
	"testing"
	"time"

	"istio.io/fortio/log"
	"istio.io/fortio/periodic"

	"google.golang.org/grpc/health/grpc_health_v1"
)

func TestGRPCRunner(t *testing.T) {
	log.SetLogLevel(log.Info)
	port := PingServer("0", false, "bar")
	destination := fmt.Sprintf("localhost:%d", port)

	opts := GRPCRunnerOptions{
		RunnerOptions: periodic.RunnerOptions{
			QPS:        100,
			Resolution: 0.00001,
		},
		Destination: destination,
		Profiler:    "test.profile",
	}
	res, err := RunGRPCTest(&opts)
	if err != nil {
		t.Error(err)
		return
	}
	totalReq := res.DurationHistogram.Count
	ok := res.RetCodes[grpc_health_v1.HealthCheckResponse_SERVING]
	if totalReq != ok {
		t.Errorf("Mismatch between requests %d and ok %v", totalReq, res.RetCodes)
	}
}

func TestGRPCRunnerWithError(t *testing.T) {
	log.SetLogLevel(log.Info)
	port := PingServer("0", false, "svc1")
	destination := fmt.Sprintf("localhost:%d", port)

	opts := GRPCRunnerOptions{
		RunnerOptions: periodic.RunnerOptions{
			QPS:      10,
			Duration: 1 * time.Second,
		},
		Destination: destination,
		Service:     "svc2",
	}
	_, err := RunGRPCTest(&opts)
	if err == nil {
		t.Error("Was expecting initial error when connecting to secure without AllowInitialErrors")
	}
	opts.AllowInitialErrors = true
	res, err := RunGRPCTest(&opts)
	if err != nil {
		t.Error(err)
		return
	}
	totalReq := res.DurationHistogram.Count
	numErrors := res.RetCodes[-1]
	if totalReq != numErrors {
		t.Errorf("Mismatch between requests %d and errors %v", totalReq, res.RetCodes)
	}
}

func TestGRPCDestination(t *testing.T) {
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
			"hostname and port",
			"localhost:1234",
			"localhost:1234",
		},
		{
			"hostname with http prefix",
			"http://localhost",
			"localhost:80",
		},
		{
			"Hostname with https prefix",
			"https://localhost",
			"localhost:443",
		},
		{
			"IPv4 address",
			"1.2.3.4",
			"1.2.3.4:8079",
		},
		{
			"IPv4 address and port",
			"1.2.3.4:5678",
			"1.2.3.4:5678",
		},
		{
			"IPv6 address",
			"2001:dba::1",
			"[2001:dba::1]:8079",
		},
		{
			"IPv6 address and port",
			"[2001:dba::1]:1234",
			"[2001:dba::1]:1234",
		},
		{
			"IPv6 address with http prefix",
			"http://2001:dba::1",
			"[2001:dba::1]:80",
		},
		{
			"IPv6 address with https prefix",
			"https://2001:dba::1",
			"[2001:dba::1]:443",
		},
	}

	for _, tc := range tests {
		dest := grpcDestination(tc.dest)
		if dest != tc.output {
			t.Errorf("Test case: %s failed to set gRPC destination\n\texpected: %s\n\t  actual: %s",
				tc.name,
				tc.output,
				dest,
			)
		}
	}
}
