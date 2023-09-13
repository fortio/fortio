// Copyright 2017 Fortio Authors.
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
	"time"

	"fortio.org/fortio/fhttp"
	"fortio.org/fortio/fnet"
	"fortio.org/log"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
)

func init() {
	log.SetLogLevel(log.Debug)
}

func TestPingServer(t *testing.T) {
	TLSSecure := &fhttp.TLSOptions{CACert: caCrt, Insecure: false}
	TLSSecureMissingCert := &fhttp.TLSOptions{Insecure: false}
	TLSSecureBadCert := &fhttp.TLSOptions{CACert: failCrt, Insecure: true}
	TLSInsecure := &fhttp.TLSOptions{Insecure: true}
	TLSInternet := &fhttp.TLSOptions{}
	iPort := PingServerTCP("0", "foo", 0, noTLSO)
	iAddr := fmt.Sprintf("localhost:%d", iPort)
	t.Logf("insecure grpc ping server running, will connect to %s", iAddr)
	sPort := PingServerTCP("0", "foo", 0, tlsO)
	sAddr := fmt.Sprintf("localhost:%d", sPort)
	t.Logf("secure grpc ping server running, will connect to %s", sAddr)
	delay := 100 * time.Millisecond
	latency, err := PingClientCall(iAddr, 7, "test payload", delay, TLSInsecure, nil)
	if err != nil || latency < delay.Seconds() || latency > 10.*delay.Seconds() {
		t.Errorf("Unexpected result %f, %v with ping calls and delay of %v", latency, err, delay)
	}
	if latency, err := PingClientCall(fnet.PrefixHTTPS+"grpc.fortio.org:443", 7, "test payload", 0,
		TLSInternet, nil); err != nil || latency <= 0 {
		t.Errorf("Unexpected result %f, %v with ping calls", latency, err)
	}
	if latency, err := PingClientCall(sAddr, 7, "test payload", 0, TLSSecure, nil); err != nil || latency <= 0 {
		t.Errorf("Unexpected result %f, %v with ping calls", latency, err)
	}
	if latency, err := PingClientCall(iAddr, 1, "", 0, TLSSecure, nil); err == nil {
		t.Errorf("Should have had an error instead of result %f for secure ping to insecure port", latency)
	}
	if _, err := PingClientCall("https://"+sAddr, 1, "", 0, TLSInsecure, nil); err != nil {
		t.Errorf("Should have had no error for secure with bad cert and insecure flag: %v", err)
	}
	if latency, err := PingClientCall("https://"+sAddr, 1, "", 0, TLSSecureMissingCert, nil); err == nil {
		t.Errorf("Should have had error for secure with bad cert and no insecure flag: %v", latency)
	}
	if latency, err := PingClientCall(sAddr, 1, "", 0, TLSInsecure, nil); err == nil {
		t.Errorf("Should have had an error instead of result %f for insecure ping to secure port", latency)
	}
	if creds, err := credentials.NewServerTLSFromFile(failCrt, failKey); err == nil {
		t.Errorf("Should have had an error instead of result %f for ping server", creds)
	}
	serving := grpc_health_v1.HealthCheckResponse_SERVING.String()
	if r, err := GrpcHealthCheck(iAddr, "", 1, TLSInsecure, nil); err != nil || (*r)[serving] != 1 {
		t.Errorf("Unexpected result %+v, %v with empty service health check", r, err)
	}
	if r, err := GrpcHealthCheck(sAddr, "", 1, TLSSecure, nil); err != nil || (*r)[serving] != 1 {
		t.Errorf("Unexpected result %+v, %v with empty service health check", r, err)
	}
	if r, err := GrpcHealthCheck(fnet.PrefixHTTPS+"grpc.fortio.org:443", "", 1, TLSInternet, nil); err != nil || (*r)[serving] != 1 {
		t.Errorf("Unexpected result %+v, %v with empty service health check", r, err)
	}
	if r, err := GrpcHealthCheck(iAddr, "foo", 3, TLSInsecure, nil); err != nil || (*r)[serving] != 3 {
		t.Errorf("Unexpected result %+v, %v with health check for same service as started (foo)", r, err)
	}
	if r, err := GrpcHealthCheck(sAddr, "foo", 3, TLSSecure, nil); err != nil || (*r)[serving] != 3 {
		t.Errorf("Unexpected result %+v, %v with health check for same service as started (foo)", r, err)
	}
	if r, err := GrpcHealthCheck(sAddr, "foo_down", 3, TLSSecure, nil); err != nil || (*r)["NOT_SERVING"] != 3 {
		t.Errorf("Unexpected result %+v, %v with health check for _down variant of same service as started (foo/foo_down)", r, err)
	}
	if r, err := GrpcHealthCheck(iAddr, "willfail", 1, TLSInsecure, nil); err == nil || r != nil {
		t.Errorf("Was expecting error when using unknown service, didn't get one, got %+v", r)
	}
	if r, err := GrpcHealthCheck(sAddr, "willfail", 1, TLSSecure, nil); err == nil || r != nil {
		t.Errorf("Was expecting error when using unknown service, didn't get one, got %+v", r)
	}
	if r, err := GrpcHealthCheck(sAddr, "willfail", 1, TLSSecureBadCert, nil); err == nil {
		t.Errorf("Was expecting dial error when using invalid certificate, didn't get one, got %+v", r)
	}
	// 2nd server on same port should fail to bind:
	newPort := PingServerTCP(strconv.Itoa(iPort), "will fail", 0, noTLSO)
	if newPort != -1 {
		t.Errorf("Didn't expect 2nd server on same port to succeed: %d %d", newPort, iPort)
	}
}

func TestDefaultHealth(t *testing.T) {
	iPort := PingServerTCP("0", "", 0, noTLSO)
	iAddr := fmt.Sprintf("localhost:%d", iPort)
	t.Logf("insecure grpc ping server running, will connect to %s", iAddr)
	serving := grpc_health_v1.HealthCheckResponse_SERVING.String()
	TLSInsecure := &fhttp.TLSOptions{Insecure: true}
	if r, err := GrpcHealthCheck(iAddr, "", 1, TLSInsecure, nil); err != nil || (*r)[serving] != 1 {
		t.Errorf("Unexpected result %+v, %v with empty service health check", r, err)
	}
	if r, err := GrpcHealthCheck(iAddr, DefaultHealthServiceName, 3, TLSInsecure, nil); err != nil || (*r)[serving] != 3 {
		t.Errorf("Unexpected result %+v, %v with health check for same service as started (ping)", r, err)
	}
	if r, err := GrpcHealthCheck(iAddr, "foo", 1, TLSInsecure, nil); err == nil || r != nil {
		t.Errorf("Was expecting error when using unknown service, didn't get one, got %+v", r)
	}
}

func TestSettingMetadata(t *testing.T) {
	server := &mdTestServer{}
	addr := server.Serve()
	TLSInsecure := &fhttp.TLSOptions{Insecure: true}
	tests := []struct {
		name      string
		key       string
		serverKey string
		value     string
	}{
		{
			name:  "valid metadata",
			key:   "abc",
			value: "def",
		},
		{
			name:  "empty value metadata",
			key:   "ghi",
			value: "",
		},
		{
			name:      "authority",
			key:       "host",
			serverKey: ":authority",
			value:     "xyz",
		},
	}

	for _, test := range tests {
		server.mdKey = test.key
		if test.serverKey != "" {
			server.mdKey = test.serverKey
		}
		server.mdValue = test.value
		_, err := PingClientCall(addr.String(), 2, "", 0, TLSInsecure, metadata.MD{
			test.key: []string{test.value},
		})
		if err != nil {
			t.Errorf("PingClientCall test case: %s failed , err: %v", test.name, err)
		}
	}

	for _, test := range tests {
		server.mdKey = test.key
		if test.serverKey != "" {
			server.mdKey = test.serverKey
		}
		server.mdValue = test.value
		_, err := GrpcHealthCheck(addr.String(), "", 2, TLSInsecure, metadata.MD{
			test.key: []string{test.value},
		})
		if err != nil {
			t.Errorf("GrpcHealthCheck test case: %s failed , err: %v", test.name, err)
		}
	}
}
