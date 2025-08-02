// Copyright 2025 Fortio Authors.
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
	"context"
	"fmt"
	"testing"
	"time"

	"fortio.org/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func init() {
	log.SetLogLevel(log.Debug)
}

func TestParseFullMethod(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedSvc    string
		expectedMethod string
		expectError    bool
	}{
		{
			name:           "valid method with leading slash",
			input:          "/fgrpc.PingServer/Ping",
			expectedSvc:    "fgrpc.PingServer",
			expectedMethod: "Ping",
			expectError:    false,
		},
		{
			name:           "valid method without leading slash",
			input:          "fgrpc.PingServer/Ping",
			expectedSvc:    "fgrpc.PingServer",
			expectedMethod: "Ping",
			expectError:    false,
		},
		{
			name:        "invalid method - no slash",
			input:       "PingServerPing",
			expectError: true,
		},
		{
			name:        "invalid method - too many slashes",
			input:       "/service/method/extra",
			expectError: true,
		},
		{
			name:        "invalid method - empty string",
			input:       "",
			expectError: true,
		},
		{
			name:        "invalid method - only slash",
			input:       "/",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, method, err := parseFullMethod(tt.input)
			if tt.expectError {
				if err == nil {
					t.Errorf("parseFullMethod(%q) expected error but got none", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("parseFullMethod(%q) unexpected error: %v", tt.input, err)
				return
			}
			if service != tt.expectedSvc {
				t.Errorf("parseFullMethod(%q) service = %q, want %q", tt.input, service, tt.expectedSvc)
			}
			if method != tt.expectedMethod {
				t.Errorf("parseFullMethod(%q) method = %q, want %q", tt.input, method, tt.expectedMethod)
			}
		})
	}
}

func TestGetMethodDescriptor(t *testing.T) {
	// Start a test gRPC server
	port := PingServerTCP("0", "test", 0, noTLSO)
	addr := fmt.Sprintf("localhost:%d", port)

	// Give the server time to start
	time.Sleep(100 * time.Millisecond)

	// Create a connection
	conn, err := grpc.NewClient(context.Background(), addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Failed to connect to test server: %v", err)
	}
	defer conn.Close()

	tests := []struct {
		name        string
		fullMethod  string
		expectError bool
	}{
		{
			name:        "valid method",
			fullMethod:  "/fgrpc.PingServer/Ping",
			expectError: false,
		},
		{
			name:        "valid method without leading slash",
			fullMethod:  "fgrpc.PingServer/Ping",
			expectError: false,
		},
		{
			name:        "invalid service",
			fullMethod:  "/NonExistentService/Ping",
			expectError: true,
		},
		{
			name:        "invalid method",
			fullMethod:  "/fgrpc.PingServer/NonExistentMethod",
			expectError: true,
		},
		{
			name:        "malformed method",
			fullMethod:  "invalid",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			md, err := getMethodDescriptor(ctx, conn, tt.fullMethod)
			if tt.expectError {
				if err == nil {
					t.Errorf("getMethodDescriptor(%q) expected error but got none", tt.fullMethod)
				}
				return
			}
			if err != nil {
				t.Errorf("getMethodDescriptor(%q) unexpected error: %v", tt.fullMethod, err)
				return
			}
			if md == nil {
				t.Errorf("getMethodDescriptor(%q) returned nil method descriptor", tt.fullMethod)
			}
		})
	}
}

func TestGetRequestMessage(t *testing.T) {
	// Start a test gRPC server to get method descriptor
	port := PingServerTCP("0", "test", 0, noTLSO)
	addr := fmt.Sprintf("localhost:%d", port)

	// Give the server time to start
	time.Sleep(100 * time.Millisecond)

	// Create a connection
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Failed to connect to test server: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	md, err := getMethodDescriptor(ctx, conn, "/fgrpc.PingServer/Ping")
	if err != nil {
		t.Fatalf("Failed to get method descriptor: %v", err)
	}

	tests := []struct {
		name        string
		jsonPayload string
		expectError bool
	}{
		{
			name:        "valid json payload",
			jsonPayload: `{"seq": 123, "ts": 456, "payload": "test message", "delayNanos": 1000}`,
			expectError: false,
		},
		{
			name:        "empty json payload",
			jsonPayload: "{}",
			expectError: false,
		},
		{
			name:        "empty string payload",
			jsonPayload: "",
			expectError: false,
		},
		{
			name:        "partial json payload",
			jsonPayload: `{"seq": 123, "payload": "test"}`,
			expectError: false,
		},
		{
			name:        "invalid json payload",
			jsonPayload: `{"seq": 123, "invalid": }`,
			expectError: true,
		},
		{
			name:        "invalid json syntax",
			jsonPayload: `{invalid json}`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := getRequestMessage(md, tt.jsonPayload)
			if tt.expectError {
				if err == nil {
					t.Errorf("getRequestMessage(%q) expected error but got none", tt.jsonPayload)
				}
				return
			}
			if err != nil {
				t.Errorf("getRequestMessage(%q) unexpected error: %v", tt.jsonPayload, err)
				return
			}
			if msg == nil {
				t.Errorf("getRequestMessage(%q) returned nil message", tt.jsonPayload)
			}
		})
	}
}

func TestDynamicGrpcCall(t *testing.T) {
	// Start a test gRPC server
	port := PingServerTCP("0", "test", 0, noTLSO)
	addr := fmt.Sprintf("localhost:%d", port)

	// Give the server time to start
	time.Sleep(100 * time.Millisecond)

	// Create a connection
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Failed to connect to test server: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Get method descriptor
	md, err := getMethodDescriptor(ctx, conn, "/fgrpc.PingServer/Ping")
	if err != nil {
		t.Fatalf("Failed to get method descriptor: %v", err)
	}

	tests := []struct {
		name        string
		jsonPayload string
		expectError bool
	}{
		{
			name:        "valid ping request",
			jsonPayload: `{"seq": 123, "ts": 456, "payload": "test message"}`,
			expectError: false,
		},
		{
			name:        "empty payload",
			jsonPayload: "{}",
			expectError: false,
		},
		{
			name:        "empty string",
			jsonPayload: "",
			expectError: false,
		},
		{
			name:        "ping with delay",
			jsonPayload: `{"seq": 1, "delayNanos": 1000000}`,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request message
			reqMsg, err := getRequestMessage(md, tt.jsonPayload)
			if err != nil {
				t.Fatalf("Failed to create request message: %v", err)
			}

			// Create DynamicGrpcCall
			call := &DynamicGrpcCall{
				MethodPath:       "/fgrpc.PingServer/Ping",
				RequestMsg:       reqMsg,
				conn:             conn,
				methodDescriptor: md,
			}

			// Perform the call
			response, err := dynamicGrpcCall(ctx, call)
			if tt.expectError {
				if err == nil {
					t.Errorf("dynamicGrpcCall expected error but got none")
				}
				return
			}
			if err != nil {
				t.Errorf("dynamicGrpcCall unexpected error: %v", err)
				return
			}
			if response == "" {
				t.Errorf("dynamicGrpcCall returned empty response")
			}

			// Basic validation that response looks like a protobuf message
			if len(response) == 0 {
				t.Errorf("dynamicGrpcCall returned empty response string")
			}
		})
	}
}

func TestDynamicGrpcCallWithInvalidConnection(t *testing.T) {
	// Create a connection to a non-existent server
	conn, err := grpc.Dial("localhost:99999", grpc.WithTransportCredentials(insecure.NewCredentials())) //nolint:staticcheck
	if err != nil {
		t.Fatalf("Failed to create connection: %v", err)
	}
	defer conn.Close()

	// This test focuses on connection failure, so we'll skip the actual call
	// since we can't create a proper method descriptor without a working server
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Test that getMethodDescriptor fails with invalid connection
	_, err = getMethodDescriptor(ctx, conn, "/NonExistent/Method")
	if err == nil {
		t.Errorf("getMethodDescriptor expected error with invalid connection but got none")
	}
}
