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
	"net"
	"reflect"
	"testing"
	"time"

	"fortio.org/fortio/fhttp"
	"fortio.org/fortio/fnet"
	"fortio.org/fortio/jrpc"
	"fortio.org/fortio/periodic"
	"fortio.org/log"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
)

var (
	// Generated from "make cert".
	caCrt  = "../cert-tmp/ca.crt"
	svrCrt = "../cert-tmp/server.crt"
	svrKey = "../cert-tmp/server.key"
	// used for failure test cases.
	failCrt = "../missing/cert.crt"
	failKey = "../missing/cert.key"
	tlsO    = &fhttp.TLSOptions{
		CACert: caCrt,
		Cert:   svrCrt,
		Key:    svrKey,
	}
	noTLSO = &fhttp.TLSOptions{}
)

func TestGRPCRunner(t *testing.T) {
	log.SetLogLevel(log.Info)
	iPort := PingServerTCP("0", "bar", 0, noTLSO)
	iDest := fmt.Sprintf("localhost:%d", iPort)
	sPort := PingServerTCP("0", "bar", 0, tlsO)
	sDest := fmt.Sprintf("localhost:%d", sPort)
	uds := fnet.GetUniqueUnixDomainPath("fortio-grpc-test")
	uPath := PingServer(uds, "", 10, noTLSO)
	uDest := "foo.bar:125"

	ro := periodic.RunnerOptions{
		QPS:        10, // some internet outcalls, not too fast
		Resolution: 0.00001,
	}

	tests := []struct {
		name       string
		runnerOpts GRPCRunnerOptions
		expect     bool
	}{
		{
			name: "valid insecure runner with payload",
			runnerOpts: GRPCRunnerOptions{
				Destination: iDest,
				Payload:     "test",
			},
			expect: true,
		},
		{
			name: "valid secure runner",
			runnerOpts: GRPCRunnerOptions{
				Destination: sDest,
				TLSOptions:  fhttp.TLSOptions{CACert: caCrt},
			},
			expect: true,
		},
		{
			name: "valid unix domain socket runner",
			runnerOpts: GRPCRunnerOptions{
				Destination: uDest,
				TLSOptions:  fhttp.TLSOptions{UnixDomainSocket: uPath.String()},
			},
			expect: true,
		},
		{
			name: "invalid insecure runner to secure server",
			runnerOpts: GRPCRunnerOptions{
				Destination: sDest,
			},
			expect: false,
		},
		{
			name: "valid secure runner using nil credentials to Internet https server",
			runnerOpts: GRPCRunnerOptions{
				Destination: "https://fortio.istio.io:443",
			},
			expect: true,
		},
		{
			name: "valid secure runner using nil credentials to Internet https server, default https port, trailing slash",
			runnerOpts: GRPCRunnerOptions{
				Destination: "https://grpc.fortio.org/",
			},
			expect: true,
		},
		{
			name: "invalid secure runner to insecure server",
			runnerOpts: GRPCRunnerOptions{
				Destination: "grpc.fortio.org:443",
			},
			expect: false,
		},
		{
			name: "invalid secure runner using test cert to https prefix Internet server",
			runnerOpts: GRPCRunnerOptions{
				Destination: "https://grpc.fortio.org:443",
				TLSOptions:  fhttp.TLSOptions{CACert: caCrt},
			},
			expect: false,
		},
		{
			name: "invalid name in secure runner cert",
			runnerOpts: GRPCRunnerOptions{
				Destination:  sDest,
				TLSOptions:   fhttp.TLSOptions{CACert: caCrt},
				CertOverride: "invalidName",
			},
			expect: false,
		},
		{
			name: "invalid cert for secure runner",
			runnerOpts: GRPCRunnerOptions{
				Destination: sDest,
				TLSOptions:  fhttp.TLSOptions{CACert: "../missing/cert.crt"},
			},
			expect: false,
		},
	}
	for _, test := range tests {
		test.runnerOpts.Profiler = "test.profile"
		test.runnerOpts.RunnerOptions = ro
		res, err := RunGRPCTest(&test.runnerOpts)
		switch {
		case err != nil && test.expect:
			t.Errorf("Test case: %s failed due to unexpected error: %v", test.name, err)
			return
		case err == nil && !test.expect:
			t.Errorf("Test case: %s failed due to unexpected response: %v", test.name, res)
			return
		case err == nil && test.expect:
			totalReq := res.DurationHistogram.Count
			ok := res.RetCodes[grpc_health_v1.HealthCheckResponse_SERVING.String()]
			if totalReq != ok {
				t.Errorf("Test case: %s failed. Mismatch between requests %d and ok %v",
					test.name, totalReq, res.RetCodes)
			}
		}
	}
}

func TestGRPCRunnerMaxStreams(t *testing.T) {
	log.SetLogLevel(log.Info)
	port := PingServerTCP("0", "maxstream", 10, noTLSO)
	destination := fmt.Sprintf("localhost:%d", port)

	opts := GRPCRunnerOptions{
		RunnerOptions: periodic.RunnerOptions{
			QPS:        100,
			NumThreads: 1,
		},
		Destination: destination,
		Streams:     10, // will be batches of 10 max
		UsePing:     true,
		Delay:       20 * time.Millisecond,
	}
	o1 := opts
	res, err := RunGRPCTest(&o1)
	if err != nil {
		t.Error(err)
		return
	}
	totalReq := res.DurationHistogram.Count
	avg10 := res.DurationHistogram.Avg
	ok := res.RetCodes[grpc_health_v1.HealthCheckResponse_SERVING.String()]
	if totalReq != ok {
		t.Errorf("Mismatch1 between requests %d and ok %v", totalReq, res.RetCodes)
	}
	if avg10 < opts.Delay.Seconds() || avg10 > 3*opts.Delay.Seconds() {
		t.Errorf("Ping delay not working, got %v for %v", avg10, opts.Delay)
	}
	o2 := opts
	o2.Streams = 20
	res, err = RunGRPCTest(&o2)
	if err != nil {
		t.Error(err)
		return
	}
	totalReq = res.DurationHistogram.Count
	avg20 := res.DurationHistogram.Avg
	ok = res.RetCodes[grpc_health_v1.HealthCheckResponse_SERVING.String()]
	if totalReq != ok {
		t.Errorf("Mismatch2 between requests %d and ok %v", totalReq, res.RetCodes)
	}
	// Half of the calls should take 2x (delayed behind maxstreams)
	if avg20 < 1.5*opts.Delay.Seconds() {
		t.Errorf("Expecting much slower average with 20/10 %v %v", avg20, avg10)
	}
}

func TestGRPCRunnerWithError(t *testing.T) {
	log.SetLogLevel(log.Info)
	iPort := PingServerTCP("0", "bar", 0, noTLSO)
	iDest := fmt.Sprintf("localhost:%d", iPort)
	sPort := PingServerTCP("0", "bar", 0, tlsO)
	sDest := fmt.Sprintf("localhost:%d", sPort)

	ro := periodic.RunnerOptions{
		QPS:      10,
		Duration: 1 * time.Second,
	}

	tests := []struct {
		name       string
		runnerOpts GRPCRunnerOptions
	}{
		{
			name: "insecure runner",
			runnerOpts: GRPCRunnerOptions{
				Destination: iDest,
			},
		},
		{
			name: "secure runner",
			runnerOpts: GRPCRunnerOptions{
				Destination: sDest,
				TLSOptions:  fhttp.TLSOptions{CACert: caCrt},
			},
		},
		{
			name: "invalid insecure runner to secure server",
			runnerOpts: GRPCRunnerOptions{
				Destination: sDest,
			},
		},
		{
			name: "invalid secure runner to insecure server",
			runnerOpts: GRPCRunnerOptions{
				Destination: iDest,
				TLSOptions:  fhttp.TLSOptions{CACert: caCrt},
			},
		},
		{
			name: "invalid name in runner cert",
			runnerOpts: GRPCRunnerOptions{
				Destination:  sDest,
				TLSOptions:   fhttp.TLSOptions{CACert: caCrt},
				CertOverride: "invalidName",
			},
		},
		{
			name: "valid runner using nil credentials to Internet https server",
			runnerOpts: GRPCRunnerOptions{
				Destination: "https://grpc.fortio.org/",
			},
		},
		{
			name: "invalid runner using test cert to https prefix Internet server",
			runnerOpts: GRPCRunnerOptions{
				Destination: "https://grpc.fortio.org/",
				TLSOptions:  fhttp.TLSOptions{CACert: caCrt},
			},
		},
		{
			name: "invalid runner using test cert to no prefix Internet server",
			runnerOpts: GRPCRunnerOptions{
				Destination: "grpc.fortio.org:443",
				TLSOptions:  fhttp.TLSOptions{CACert: caCrt},
			},
		},
	}
	for _, test := range tests {
		test.runnerOpts.Service = "svc2"
		test.runnerOpts.RunnerOptions = ro
		_, err := RunGRPCTest(&test.runnerOpts)
		if err == nil {
			t.Error("Was expecting initial error when connecting to secure without AllowInitialErrors")
		}
		test.runnerOpts.AllowInitialErrors = true
		res, err := RunGRPCTest(&test.runnerOpts)
		if err != nil {
			t.Errorf("Test case: %s failed due to unexpected error: %v", test.name, err)
			return
		}
		totalReq := res.DurationHistogram.Count
		numErrors := res.RetCodes[Error]
		if totalReq != numErrors {
			t.Errorf("Test case: %s failed. Mismatch between requests %d and errors %v",
				test.name, totalReq, res.RetCodes)
		}
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
			"Hostname with http prefix and trailing /",
			"http://localhost/",
			"localhost:80",
		},
		{
			"Hostname with https prefix",
			"https://localhost",
			"localhost:443",
		},
		{
			"Hostname with https prefix and trailing /",
			"https://localhost/",
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
			"IPv4 address with http prefix and trailing /",
			"http://1.2.3.4/",
			"1.2.3.4:80",
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
		{
			"IPv6 address with https prefix and trailing /",
			"https://2001:dba::1/",
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

type mdTestServer struct {
	t       *testing.T
	mdKey   string
	mdValue string
	health.Server
}

func (m *mdTestServer) Ping(ctx context.Context, _ *PingMessage) (*PingMessage, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	val := md.Get(m.mdKey)

	if len(val) == 0 || val[0] != m.mdValue {
		m.t.Errorf("metadata %s not found or value is not %s,actual value: %v", m.mdKey, m.mdValue, val)
	}
	return &PingMessage{}, nil
}

func (m *mdTestServer) Check(ctx context.Context, _ *grpc_health_v1.HealthCheckRequest,
) (*grpc_health_v1.HealthCheckResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	val := md.Get(m.mdKey)
	if len(val) == 0 || val[0] != m.mdValue {
		m.t.Errorf("metadata %q not found or value is not %q, actual value: %v", m.mdKey, m.mdValue, val)
	}
	return &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING}, nil
}

func (m *mdTestServer) Serve() *net.TCPAddr {
	server := grpc.NewServer()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		m.t.Fatal(err)
		return nil
	}
	grpc_health_v1.RegisterHealthServer(server, m)
	RegisterPingServerServer(server, m)
	go func() {
		err = server.Serve(lis)
		if err != nil {
			panic(err)
		}
	}()
	return lis.Addr().(*net.TCPAddr)
}

func TestGRPCRunnerWithMetadata(t *testing.T) {
	server := &mdTestServer{t: t}
	addr := server.Serve()
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
		_, err := RunGRPCTest(&GRPCRunnerOptions{
			Streams:     10,
			Destination: addr.String(),
			Metadata: map[string][]string{
				test.key: {test.value},
			},
		})
		if err != nil {
			t.Errorf("Test case: %s failed, err: %v", test.name, err)
		}
		// errors will be set by the server
	}
}

func TestHeaderHandling(t *testing.T) {
	type args struct {
		in metadata.MD
	}
	tests := []struct {
		name       string
		args       args
		wantOutLen int
		wantMD     metadata.MD
	}{
		{
			name: "host",
			args: args{
				in: map[string][]string{
					"user-key": {"value"},
					"host":     {"a.b"},
				},
			},
			wantOutLen: 1,
			wantMD: map[string][]string{
				"user-key": {"value"},
			},
		},
		{
			name: "user-agent",
			args: args{
				in: map[string][]string{
					"user-agent": {"value"},
					"host":       {"a.b"},
				},
			},
			wantOutLen: 2,
			wantMD:     map[string][]string{},
		},
		{
			name: "user-agent2",
			args: args{
				in: map[string][]string{
					"user-agent": {jrpc.UserAgent},
					"host":       {"a.b"},
				},
			},
			wantOutLen: 2,
			wantMD:     map[string][]string{},
		},
		{
			name: "user-key",
			args: args{
				in: map[string][]string{
					"user-key": {"value"},
				},
			},
			wantOutLen: 0,
			wantMD: map[string][]string{
				"user-key": {"value"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOut, mdOut := extractDialOptionsAndFilter(tt.args.in)
			if !reflect.DeepEqual(len(gotOut), tt.wantOutLen) {
				t.Errorf("extractDialOptionsAndFilter() = %v, want %v", len(gotOut), tt.wantOutLen)
			}
			if !reflect.DeepEqual(mdOut, tt.wantMD) {
				t.Errorf("got md = %v, want %v", tt.args.in, tt.wantMD)
			}
		})
	}
}
