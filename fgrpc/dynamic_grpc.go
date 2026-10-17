package fgrpc

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"fortio.org/log"
	"github.com/jhump/protoreflect/v2/grpcdynamic"
	"github.com/jhump/protoreflect/v2/grpcreflect"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// DynamicGrpcCall represents a prepared dynamic gRPC call with all necessary components.
type DynamicGrpcCall struct {
	MethodPath string        // e.g. "/Service/Method"
	RequestMsg proto.Message // dynamic protobuf for request

	conn             *grpc.ClientConn              // gRPC connection to use for the call
	methodDescriptor protoreflect.MethodDescriptor // Method descriptor for the gRPC method
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

	if call.methodDescriptor == nil {
		return "", errors.New("method descriptor is nil")
	}

	stub := grpcdynamic.NewStub(call.conn)
	response, err := stub.InvokeRPC(ctx, call.methodDescriptor, call.RequestMsg)
	if err != nil {
		return "", fmt.Errorf("gRPC invoke error: %w", err)
	}
	log.Debugf("gRpc response: %v", response)
	return prototext.Format(response), nil
}

// getMethodDescriptor retrieves the method descriptor for a given full method name.
func getMethodDescriptor(ctx context.Context, conn *grpc.ClientConn, fullMethod string) (protoreflect.MethodDescriptor, error) {
	refClient := grpcreflect.NewClientAuto(ctx, conn)
	defer refClient.Reset()

	serviceName, methodName, err := parseFullMethod(fullMethod)
	if err != nil {
		return nil, err
	}

	fileDesc, err := refClient.FileContainingSymbol(protoreflect.FullName(serviceName))
	if err != nil {
		return nil, err
	}

	var sd protoreflect.ServiceDescriptor
	services := fileDesc.Services()
	for i := 0; i < services.Len(); i++ {
		s := services.Get(i)
		if string(s.FullName()) == serviceName || string(s.Name()) == serviceName {
			sd = s
			break
		}
	}
	if sd == nil {
		return nil, fmt.Errorf("service %s not found", serviceName)
	}

	md := sd.Methods().ByName(protoreflect.Name(methodName))
	if md == nil {
		return nil, fmt.Errorf("method %s not found in service %s", methodName, serviceName)
	}

	log.Debugf("Resolved method descriptor for %s: %s", fullMethod, md.FullName())
	return md, nil
}

// getRequestMessage creates a protobuf message from the JSON payload.
func getRequestMessage(md protoreflect.MethodDescriptor, jsonPayload string) (proto.Message, error) {
	inputType := md.Input()
	inMsg := dynamicpb.NewMessage(inputType)
	if jsonPayload == "" || jsonPayload == "{}" {
		log.Debugf("Empty payload. Creating empty message for %s", inputType.FullName())
		// return empty message if no payload
		return inMsg, nil
	}

	if err := protojson.Unmarshal([]byte(jsonPayload), inMsg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON payload: %w", err)
	}
	log.Debugf("Created request message for %s: %s", inputType.FullName(), inMsg)
	return inMsg, nil
}
