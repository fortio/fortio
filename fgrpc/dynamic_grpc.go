package fgrpc // import "fortio.org/fortio/fgrpc"

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"fortio.org/log"
	"google.golang.org/grpc"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/jhump/protoreflect/dynamic/grpcdynamic"
	"github.com/jhump/protoreflect/grpcreflect"
)

// DynamicGrpcCall represents a prepared dynamic gRPC call with all necessary components.
type DynamicGrpcCall struct {
	MethodPath  string           // e.g. "/Service/Method"
	RequestMsg  *dynamic.Message // dynamic protobuf for request

	conn             *grpc.ClientConn       // gRPC connection to use for the call
	methodDescriptor *desc.MethodDescriptor // Method descriptor for the gRPC method
}


// parseFullMethod splits "Service/Method" or "/Service/Method" into service and method.
func parseFullMethod(full string) (string, string, error) {
	trimmed := strings.TrimPrefix(full, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid method format: want Service/Method, got %q", full)
	}
	return parts[0], parts[1], nil
}

// dynamicGrpcCall performs a dynamic unary gRPC call.
func dynamicGrpcCall(ctx context.Context, call *DynamicGrpcCall) (string, error) {
	log.Debugf("Invoking gRPC method %s with input: %s", call.MethodPath, call.RequestMsg)

	stub := grpcdynamic.NewStub(call.conn)
	response, err := stub.InvokeRpc(ctx, call.methodDescriptor, call.RequestMsg)
	if err != nil {
		return "", fmt.Errorf("gRPC invoke error: %w", err)
	}
	log.Debugf("gRpc response: %v", response)
	return response.String(), nil
}

// getMethodDescriptor retrieves the method descriptor for a given full method name
func getMethodDescriptor(ctx context.Context, conn *grpc.ClientConn, fullMethod string) (*desc.MethodDescriptor, error) {
	refClient := grpcreflect.NewClientAuto(ctx, conn)
	defer refClient.Reset()

	serviceName, methodName, err := parseFullMethod(fullMethod)
	if err != nil {
		return nil, err
	}

	sd, err := refClient.ResolveService(serviceName)
	if err != nil {
		return nil, err
	}
	md := sd.FindMethodByName(methodName)
	if md == nil {
		return nil, fmt.Errorf("method %s not found in service %s", methodName, serviceName)
	}

	log.Debugf("Resolved method descriptor for %s: %s", fullMethod, md.GetFullyQualifiedName())
	return md, nil
}

// getRequestMessage creates a protobuf message from the JSON payload
func getRequestMessage(md *desc.MethodDescriptor, jsonPayload string) (*dynamic.Message, error) {
	inputType := md.GetInputType()
	inMsg := dynamic.NewMessage(inputType)
	if jsonPayload == "" || jsonPayload == "{}" {
		log.Debugf("Empty payload. Creating empty message for %s", inputType.GetFullyQualifiedName())
		// return empty message if no payload
		return inMsg, nil
	}

	if err := json.Unmarshal([]byte(jsonPayload), inMsg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON payload: %w", err)
	}
	log.Debugf("Created request message for %s: %s", inputType, inMsg)
	return inMsg, nil
}