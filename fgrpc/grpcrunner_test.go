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
	"net"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"fortio.org/fortio/fhttp"
	"fortio.org/fortio/fnet"
	"fortio.org/fortio/jrpc"
	"fortio.org/fortio/periodic"
	"fortio.org/log"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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

// TestGRPCRunnerCustomMethod tests custom gRPC method load testing functionality.
func TestGRPCRunnerCustomMethod(t *testing.T) {
	log.SetLogLevel(log.Info)

	// Start a test gRPC server
	port := PingServerTCP("0", "custom-method-test", 0, noTLSO)
	addr := fmt.Sprintf("localhost:%d", port)

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	tests := []struct {
		name            string
		grpcMethod      string
		payload         string
		numThreads      int
		expectError     bool
		expectedRunType string
	}{
		{
			name:            "valid custom method with payload",
			grpcMethod:      "fgrpc.PingServer/Ping",
			payload:         `{"seq": 1, "payload": "test", "ts": 123456789}`,
			numThreads:      1,
			expectError:     false,
			expectedRunType: "Custom GRPC Method fgrpc.PingServer/Ping and Payload",
		},
		{
			name:            "valid custom method with empty payload",
			grpcMethod:      "fgrpc.PingServer/Ping",
			payload:         "",
			numThreads:      1,
			expectError:     false,
			expectedRunType: "Custom GRPC Method fgrpc.PingServer/Ping and Payload",
		},
		{
			name:            "valid custom method with multiple threads",
			grpcMethod:      "fgrpc.PingServer/Ping",
			payload:         `{"seq": 42}`,
			numThreads:      2,
			expectError:     false,
			expectedRunType: "Custom GRPC Method fgrpc.PingServer/Ping and Payload",
		},
		{
			name:        "invalid method name",
			grpcMethod:  "NonExistent/Method",
			payload:     "{}",
			numThreads:  1,
			expectError: true,
		},
		{
			name:        "invalid method format",
			grpcMethod:  "InvalidFormat",
			payload:     "{}",
			numThreads:  1,
			expectError: true,
		},
		{
			name:        "invalid JSON payload",
			grpcMethod:  "fgrpc.PingServer/Ping",
			payload:     `{"invalid": json}`,
			numThreads:  1,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := &GRPCRunnerOptions{
				RunnerOptions: periodic.RunnerOptions{
					QPS:        10,
					Duration:   200 * time.Millisecond,
					NumThreads: tt.numThreads,
					Out:        os.Stderr,
				},
				Destination: addr,
				GrpcMethod:  tt.grpcMethod,
				Payload:     tt.payload,
			}

			res, err := RunGRPCTest(o)

			if tt.expectError {
				if err == nil {
					t.Errorf("RunGRPCTest expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("RunGRPCTest unexpected error: %v", err)
				return
			}

			if res == nil {
				t.Errorf("RunGRPCTest returned nil result")
				return
			}

			// Check that the run type is set correctly
			if !strings.Contains(o.RunType, tt.expectedRunType) {
				t.Errorf("RunGRPCTest RunType = %q, want to contain %q", o.RunType, tt.expectedRunType)
			}

			// Check that results contain some data
			if len(res.RetCodes) == 0 {
				t.Errorf("RunGRPCTest returned no return codes")
			}

			// Check that there are some successful calls
			if res.RetCodes["SERVING"] == 0 {
				t.Errorf("RunGRPCTest returned no successful calls: %+v", res.RetCodes)
			}
		})
	}
}

// TestGRPCRunnerCustomMethodErrors tests error handling in custom gRPC method execution.
func TestGRPCRunnerCustomMethodErrors(t *testing.T) {
	log.SetLogLevel(log.Info)

	// Start a test gRPC server
	port := PingServerTCP("0", "error-test", 0, noTLSO)
	addr := fmt.Sprintf("localhost:%d", port)

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Test method descriptor error
	t.Run("method descriptor error", func(t *testing.T) {
		o := &GRPCRunnerOptions{
			RunnerOptions: periodic.RunnerOptions{
				QPS:        1,
				Duration:   100 * time.Millisecond,
				NumThreads: 1,
				Out:        os.Stderr,
			},
			Destination: addr,
			GrpcMethod:  "NonExistent/Method",
			Payload:     "{}",
		}

		_, err := RunGRPCTest(o)
		if err == nil {
			t.Errorf("RunGRPCTest expected error for non-existent method but got none")
		}

		if !strings.Contains(err.Error(), "failed to get method descriptor") {
			t.Errorf("RunGRPCTest error = %q, want to contain 'failed to get method descriptor'", err.Error())
		}
	})

	// Test request message error
	t.Run("request message error", func(t *testing.T) {
		o := &GRPCRunnerOptions{
			RunnerOptions: periodic.RunnerOptions{
				QPS:        1,
				Duration:   100 * time.Millisecond,
				NumThreads: 1,
				Out:        os.Stderr,
			},
			Destination: addr,
			GrpcMethod:  "fgrpc.PingServer/Ping",
			Payload:     `{"invalid": json}`,
		}

		_, err := RunGRPCTest(o)
		if err == nil {
			t.Errorf("RunGRPCTest expected error for invalid JSON payload but got none")
		}

		if !strings.Contains(err.Error(), "failed to get request message") {
			t.Errorf("RunGRPCTest error = %q, want to contain 'failed to get request message'", err.Error())
		}
	})
}

// TestGRPCRunnerCustomMethodWithConnection tests custom gRPC method with connection issues.
func TestGRPCRunnerCustomMethodWithConnection(t *testing.T) {
	log.SetLogLevel(log.Info)

	// Test with non-existent server
	t.Run("connection error", func(t *testing.T) {
		o := &GRPCRunnerOptions{
			RunnerOptions: periodic.RunnerOptions{
				QPS:        1,
				Duration:   100 * time.Millisecond,
				NumThreads: 1,
				Out:        os.Stderr,
			},
			Destination: "localhost:99999", // non-existent server
			GrpcMethod:  "fgrpc.PingServer/Ping",
			Payload:     "{}",
		}

		_, err := RunGRPCTest(o)
		if err == nil {
			t.Errorf("RunGRPCTest expected error for non-existent server but got none")
		}
	})
}

// TestDynamicGrpcCallErrors tests error handling in dynamicGrpcCall function.
func TestDynamicGrpcCallErrors(t *testing.T) {
	log.SetLogLevel(log.Info)

	// Test with closed connection
	t.Run("closed connection error", func(t *testing.T) {
		// Create a connection and close it
		conn, err := grpc.NewClient("localhost:99999", grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			t.Fatalf("Failed to create connection: %v", err)
		}
		conn.Close()

		// Try to create a call with closed connection
		call := &DynamicGrpcCall{
			MethodPath: "fgrpc.PingServer/Ping",
			conn:       conn,
		}

		ctx := context.Background()
		_, err = dynamicGrpcCall(ctx, call)
		if err == nil {
			t.Errorf("dynamicGrpcCall expected error with closed connection but got none")
		}
	})

	// Test with invalid method descriptor
	t.Run("nil method descriptor", func(t *testing.T) {
		conn, err := grpc.NewClient("localhost:99999", grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			t.Fatalf("Failed to create connection: %v", err)
		}
		defer conn.Close()

		call := &DynamicGrpcCall{
			MethodPath:       "fgrpc.PingServer/Ping",
			conn:             conn,
			methodDescriptor: nil, // nil descriptor
		}

		ctx := context.Background()
		_, err = dynamicGrpcCall(ctx, call)
		if err == nil {
			t.Errorf("dynamicGrpcCall expected error with nil method descriptor but got none")
		}
	})
}
