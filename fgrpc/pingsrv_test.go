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

package fgrpc

import (
	"fmt"
	"strconv"
	"testing"

	"google.golang.org/grpc/health/grpc_health_v1"

	"istio.io/fortio/log"
)

func init() {
	log.SetLogLevel(log.Debug)
}

func TestPingServer(t *testing.T) {
	iPort := PingServer("0", "", "", "foo")
	iAddr := fmt.Sprintf("localhost:%d", iPort)
	t.Logf("test insecure grpc ping server running, will connect to %s", iAddr)
	sPort := PingServer("0", "../testdata/server.crt", "../testdata/server.key",
		"foo")
	sAddr := fmt.Sprintf("localhost:%d", sPort)
	t.Logf("test secure grpc ping server running, will connect to %s", sAddr)
	if latency, err := PingClientCall(iAddr, "", 7, "test payload"); err != nil || latency <= 0 {
		t.Errorf("Unexpected result %f, %v with ping calls", latency, err)
	}
	if latency, err := PingClientCall(prefixHTTPS+"fortio.istio.io:443", "", 7, "test payload"); err != nil || latency <= 0 {
		t.Errorf("Unexpected result %f, %v with ping calls", latency, err)
	}
	if latency, err := PingClientCall(sAddr, svrCrt, 7, "test payload"); err != nil || latency <= 0 {
		t.Errorf("Unexpected result %f, %v with ping calls", latency, err)
	}
	if latency, err := PingClientCall(iAddr, svrCrt, 1, ""); err == nil {
		t.Errorf("Should have had an error instead of result %f for secure ping to insecure port", latency)
	}
	if latency, err := PingClientCall(sAddr, "", 1, ""); err == nil {
		t.Errorf("Should have had an error instead of result %f for insecure ping to secure port", latency)
	}
	serving := grpc_health_v1.HealthCheckResponse_SERVING
	if r, err := GrpcHealthCheck(iAddr, "", "", 1); err != nil || (*r)[serving] != 1 {
		t.Errorf("Unexpected result %+v, %v with empty service health check", r, err)
	}
	if r, err := GrpcHealthCheck(sAddr, svrCrt, "", 1); err != nil || (*r)[serving] != 1 {
		t.Errorf("Unexpected result %+v, %v with empty service health check", r, err)
	}
	if r, err := GrpcHealthCheck(prefixHTTPS+"fortio.istio.io:443", "", "", 1); err != nil || (*r)[serving] != 1 {
		t.Errorf("Unexpected result %+v, %v with empty service health check", r, err)
	}
	if r, err := GrpcHealthCheck(iAddr, "", "foo", 3); err != nil || (*r)[serving] != 3 {
		t.Errorf("Unexpected result %+v, %v with health check for same service as started (foo)", r, err)
	}
	if r, err := GrpcHealthCheck(sAddr, svrCrt, "foo", 3); err != nil || (*r)[serving] != 3 {
		t.Errorf("Unexpected result %+v, %v with health check for same service as started (foo)", r, err)
	}
	if r, err := GrpcHealthCheck(iAddr, "", "willfail", 1); err == nil || r != nil {
		t.Errorf("Was expecting error when using unknown service, didn't get one, got %+v", r)
	}
	if r, err := GrpcHealthCheck(sAddr, svrCrt, "willfail", 1); err == nil || r != nil {
		t.Errorf("Was expecting error when using unknown service, didn't get one, got %+v", r)
	}
	// 2nd server on same port should fail to bind:
	newPort := PingServer(strconv.Itoa(iPort), "", "", "will fail")
	if newPort != -1 {
		t.Errorf("Didn't expect 2nd server on same port to succeed: %d %d", newPort, iPort)
	}
}
