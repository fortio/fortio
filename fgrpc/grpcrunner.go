// Copyright 2017 Fortio Authors
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

package fgrpc // import "fortio.org/fortio/fgrpc"

import (
	"context"
	"fmt"
	"net"
	"os"
	"runtime"
	"runtime/pprof"
	"strings"
	"time"

	"fortio.org/fortio/fhttp"
	"fortio.org/fortio/fnet"
	"fortio.org/fortio/periodic"
	"fortio.org/log"
	"github.com/jhump/protoreflect/desc"    //nolint:staticcheck // TODO: migrate to v2 API
	"github.com/jhump/protoreflect/dynamic" //nolint:staticcheck // TODO: migrate to v2 API
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
)

// Dial dials gRPC using insecure or TLS transport security when serverAddr
// has prefixHTTPS or cert is provided. If override is set to a non-empty string,
// it will override the virtual host name of authority in requests.
func Dial(o *GRPCRunnerOptions) (*grpc.ClientConn, error) {
	var opts []grpc.DialOption
	if o.CACert != "" || strings.HasPrefix(o.Destination, fnet.PrefixHTTPS) {
		tlsConfig, err := o.TLSOptions.TLSConfig()
		if err != nil {
			return nil, err
		}
		tlsConfig.ServerName = o.CertOverride
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	serverAddr := grpcDestination(o.Destination)
	if o.UnixDomainSocket != "" {
		log.Warnf("Using domain socket %v instead of %v for grpc connection", o.UnixDomainSocket, serverAddr)
		opts = append(opts, grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			dialer := net.Dialer{}
			return dialer.DialContext(ctx, fnet.UnixDomainSocket, o.UnixDomainSocket)
		}))
	}
	opts = append(opts, o.dialOptions...)
	conn, err := grpc.Dial(serverAddr, opts...) //nolint:staticcheck // NewClient() makes the tests fail, not sure why
	if err != nil {
		log.Errf("failed to connect to %s with certificate %s and override %s: %v", serverAddr, o.CACert, o.CertOverride, err)
	}
	return conn, err
}

// TODO: refactor common parts between HTTP and gRPC runners.

// GRPCRunnerResults is the aggregated result of an GRPCRunner.
// Also is the internal type used per thread/goroutine.
type GRPCRunnerResults struct {
	periodic.RunnerResults
	clientH     grpc_health_v1.HealthClient
	reqH        grpc_health_v1.HealthCheckRequest
	clientP     PingServerClient
	reqP        PingMessage
	RetCodes    HealthResultMap
	Destination string
	Streams     int
	Ping        bool
	Metadata    metadata.MD
	dynamicCall *DynamicGrpcCall
}

// Run exercises GRPC health check or ping at the target QPS.
// To be set as the Function in RunnerOptions.
func (grpcstate *GRPCRunnerResults) Run(outCtx context.Context, t periodic.ThreadID) (bool, string) {
	log.Debugf("Calling in %d", t)
	var err error
	var res interface{}
	status := grpc_health_v1.HealthCheckResponse_SERVING
	if len(grpcstate.Metadata) != 0 { // filtered one
		outCtx = metadata.NewOutgoingContext(outCtx, grpcstate.Metadata)
	}
	switch {
	case grpcstate.Ping:
		res, err = grpcstate.clientP.Ping(outCtx, &grpcstate.reqP)
	case grpcstate.dynamicCall != nil:
		res, err = dynamicGrpcCall(outCtx, grpcstate.dynamicCall)
		if err != nil {
			log.Warnf("Error making dynamic gRPC call: %v", err)
			grpcstate.RetCodes[Error]++
		}
		log.Debugf("Dynamic gRPC call response: %s, error: %v", res, err)
	default:
		var r *grpc_health_v1.HealthCheckResponse
		r, err = grpcstate.clientH.Check(outCtx, &grpcstate.reqH)
		if r != nil {
			status = r.GetStatus()
			res = r
		}
	}
	log.Debugf("For %d (ping=%v) got %v %v", t, grpcstate.Ping, err, res)
	if err != nil {
		log.Warnf("Error making grpc call: %v", err)
		grpcstate.RetCodes[Error]++
		return false, err.Error()
	}
	grpcstate.RetCodes[status.String()]++
	if status == grpc_health_v1.HealthCheckResponse_SERVING {
		return true, "SERVING"
	}
	return false, status.String()
}

// GRPCRunnerOptions includes the base RunnerOptions plus gRPC specific
// options.
type GRPCRunnerOptions struct {
	periodic.RunnerOptions
	fhttp.TLSOptions
	Destination        string
	Service            string            // Service to be checked when using gRPC health check
	Profiler           string            // file to save profiles to. defaults to no profiling
	Payload            string            // Payload to be sent for gRPC ping service
	Streams            int               // number of streams. total go routines and data streams will be streams*numthreads.
	Delay              time.Duration     // Delay to be sent when using gRPC ping service
	CertOverride       string            // Override the cert virtual host of authority for testing
	AllowInitialErrors bool              // whether initial errors don't cause an abort
	UsePing            bool              // use our own Ping proto for gRPC load instead of standard health check one.
	Metadata           metadata.MD       // input metadata that will be added to the request
	dialOptions        []grpc.DialOption // gRPC dial options extracted from Metadata (authority and user-agent extracted)
	filteredMetadata   metadata.MD       // filtered version of Metadata metadata (without authority and user-agent)
	GrpcCompression    bool              // enable gRPC compression
	GrpcMethod         string            // gRPC method to call (Service/Method)
}

// RunGRPCTest runs an HTTP test and returns the aggregated stats.
//
//nolint:funlen, gocognit, gocyclo // yes it's long.
func RunGRPCTest(o *GRPCRunnerOptions) (*GRPCRunnerResults, error) {
	if o.Streams < 1 {
		o.Streams = 1
	}
	if o.NumThreads < 1 {
		// sort of todo, this redoing some of periodic normalize (but we can't use normalize which does too much)
		o.NumThreads = periodic.DefaultRunnerOptions.NumThreads
	}
	switch {
	case o.UsePing:
		o.RunType = "GRPC Ping"
		if o.Delay > 0 {
			o.RunType += fmt.Sprintf(" Delay=%v", o.Delay)
		}
	case o.GrpcMethod != "":
		o.RunType = fmt.Sprintf("Custom GRPC Method %s and Payload %s", o.GrpcMethod, o.Payload)
	default:
		o.RunType = "GRPC Health for '" + o.Service + "'"
	}
	pll := len(o.Payload)
	if pll > 0 {
		o.RunType += fmt.Sprintf(" PayloadLength=%d", pll)
	}
	log.Infof("Starting %s test for %s with %d*%d threads at %.1f qps, compression: %v",
		o.RunType, o.Destination, o.Streams, o.NumThreads, o.QPS, o.GrpcCompression)
	o.NumThreads *= o.Streams
	r := periodic.NewPeriodicRunner(&o.RunnerOptions)
	defer r.Options().Abort()
	numThreads := r.Options().NumThreads // may change
	o.dialOptions, o.filteredMetadata = extractDialOptionsAndFilter(o.Metadata)

	callOptions := make([]grpc.CallOption, 0)
	if o.GrpcCompression {
		callOptions = append(callOptions, grpc.UseCompressor(gzip.Name))
	}

	total := GRPCRunnerResults{
		RetCodes:    make(HealthResultMap),
		Destination: o.Destination,
		Streams:     o.Streams,
		Ping:        o.UsePing,
		Metadata:    o.Metadata, // the original one
	}
	grpcstate := make([]GRPCRunnerResults, numThreads)
	out := r.Options().Out // Important as the default value is set from nil to stdout inside NewPeriodicRunner
	var conn *grpc.ClientConn
	var err error
	var methodDescriptor *desc.MethodDescriptor
	var reqMsg *dynamic.Message
	ts := time.Now().UnixNano()
	for i := range numThreads {
		r.Options().Runners[i] = &grpcstate[i]
		newConn := i%o.Streams == 0
		if newConn {
			conn, err = Dial(o)
			if err != nil {
				log.Errf("Error in grpc dial for %s %v", o.Destination, err)
				return nil, err
			}
		} else {
			log.Debugf("Reusing previous client connection for %d", i)
		}
		grpcstate[i].Ping = o.UsePing
		var err error
		outCtx := context.Background()
		if o.filteredMetadata.Len() != 0 {
			outCtx = metadata.NewOutgoingContext(outCtx, o.filteredMetadata)
			grpcstate[i].Metadata = o.filteredMetadata // the one used to send
		}
		// TODO: support parallel warmup(implemented in http)
		switch {
		case o.UsePing:
			grpcstate[i].clientP = NewPingServerClient(conn)
			if grpcstate[i].clientP == nil {
				return nil, fmt.Errorf("unable to create ping client %d for %s", i, o.Destination)
			}
			grpcstate[i].reqP = PingMessage{Payload: o.Payload, DelayNanos: o.Delay.Nanoseconds(), Seq: int64(i), Ts: ts}
			if newConn && o.Exactly <= 0 {
				_, err = grpcstate[i].clientP.Ping(outCtx, &grpcstate[i].reqP, callOptions...)
			}
		case o.GrpcMethod != "":
			// Use reflection to get method descriptor and create request message, if not already done.
			// these can be reused across threads
			if methodDescriptor == nil {
				methodDescriptor, err = getMethodDescriptor(outCtx, conn, o.GrpcMethod)
				if err != nil {
					return nil, fmt.Errorf("failed to get method descriptor for %s: %w", o.GrpcMethod, err)
				}
			}
			if reqMsg == nil {
				reqMsg, err = getRequestMessage(methodDescriptor, o.Payload)
				if err != nil {
					return nil, fmt.Errorf("failed to get request message for %s: %w", o.GrpcMethod, err)
				}
			}
			grpcstate[i].dynamicCall = &DynamicGrpcCall{
				methodDescriptor: methodDescriptor,
				conn:             conn,
				MethodPath:       o.GrpcMethod,
				RequestMsg:       reqMsg,
			}
		default:
			grpcstate[i].clientH = grpc_health_v1.NewHealthClient(conn)
			if grpcstate[i].clientH == nil {
				return nil, fmt.Errorf("unable to create health client %d for %s", i, o.Destination)
			}
			grpcstate[i].reqH = grpc_health_v1.HealthCheckRequest{Service: o.Service}
			if newConn && o.Exactly <= 0 {
				_, err = grpcstate[i].clientH.Check(outCtx, &grpcstate[i].reqH)
			}
		}
		if !o.AllowInitialErrors && err != nil {
			log.Errf("Error in first grpc call (ping = %v) for %s: %v", o.UsePing, o.Destination, err)
			return nil, err
		}
		// Setup the stats for each 'thread'
		grpcstate[i].RetCodes = make(HealthResultMap)
	}

	if o.Profiler != "" {
		fc, err := os.Create(o.Profiler + ".cpu")
		if err != nil {
			log.Critf("Unable to create .cpu profile: %v", err)
			return nil, err
		}
		if err = pprof.StartCPUProfile(fc); err != nil {
			log.Critf("Unable to start cpu profile: %v", err)
		}
	}
	total.RunnerResults = r.Run()
	if o.Profiler != "" {
		pprof.StopCPUProfile()
		fm, err := os.Create(o.Profiler + ".mem")
		if err != nil {
			log.Critf("Unable to create .mem profile: %v", err)
			return nil, err
		}
		runtime.GC() // get up-to-date statistics
		if err = pprof.WriteHeapProfile(fm); err != nil {
			log.Critf("Unable to write heap profile: %v", err)
		}
		fm.Close()
		fmt.Printf("Wrote profile data to %s.{cpu|mem}\n", o.Profiler)
	}
	// Numthreads may have reduced
	numThreads = r.Options().NumThreads
	keys := []string{}
	for i := range numThreads {
		// Q: is there some copying each time stats[i] is used?
		for k := range grpcstate[i].RetCodes {
			if _, exists := total.RetCodes[k]; !exists {
				keys = append(keys, k)
			}
			total.RetCodes[k] += grpcstate[i].RetCodes[k]
		}
		// TODO: if gRPC client needs 'cleanup'/Close like HTTP one, do it on original NumThreads
	}
	// Cleanup state:
	r.Options().ReleaseRunners()
	which := "Health"
	if o.UsePing {
		which = "Ping"
	} else if o.GrpcMethod != "" {
		which = "Custom gRPC Method"
	}
	_, _ = fmt.Fprintf(out, "Jitter: %t\n", total.Jitter)
	for _, k := range keys {
		_, _ = fmt.Fprintf(out, "%s %s : %d\n", which, k, total.RetCodes[k])
	}
	return &total, nil
}

// grpcDestination parses dest and returns dest:port based on dest being
// a hostname, IP address, hostname:port, or ip:port. The original dest is
// returned if dest is an invalid hostname or invalid IP address. An http/https
// prefix is removed from dest if one exists and the port number is set to
// StandardHTTPPort for http, StandardHTTPSPort for https, or DefaultGRPCPort
// if http, https, or :port is not specified in dest.
// TODO: change/fix this (NormalizePort and more).
func grpcDestination(dest string) (parsedDest string) {
	var port string
	// strip any unintentional http/https scheme prefixes from dest
	// and set the port number.
	switch {
	case strings.HasPrefix(dest, fnet.PrefixHTTP):
		parsedDest = strings.TrimSuffix(strings.Replace(dest, fnet.PrefixHTTP, "", 1), "/")
		port = fnet.StandardHTTPPort
		log.Infof("stripping http scheme. grpc destination: %v: grpc port: %s",
			parsedDest, port)
	case strings.HasPrefix(dest, fnet.PrefixHTTPS):
		parsedDest = strings.TrimSuffix(strings.Replace(dest, fnet.PrefixHTTPS, "", 1), "/")
		port = fnet.StandardHTTPSPort
		log.Infof("stripping https scheme. grpc destination: %v. grpc port: %s",
			parsedDest, port)
	default:
		parsedDest = dest
		port = fnet.DefaultGRPCPort
	}
	if _, _, err := net.SplitHostPort(parsedDest); err == nil {
		return parsedDest
	}
	if ip := net.ParseIP(parsedDest); ip != nil {
		switch {
		case ip.To4() != nil:
			parsedDest = ip.String() + fnet.NormalizePort(port)
			return parsedDest
		case ip.To16() != nil:
			parsedDest = "[" + ip.String() + "]" + fnet.NormalizePort(port)
			return parsedDest
		}
	}
	// parsedDest is in the form of a domain name,
	// append ":port" and return.
	parsedDest += fnet.NormalizePort(port)
	return parsedDest
}

// extractDialOptionsAndFilter converts special MD into dial options and filters them in outMD.
func extractDialOptionsAndFilter(in metadata.MD) (out []grpc.DialOption, outMD metadata.MD) {
	outMD = make(metadata.MD, len(in))
	for k, v := range in {
		switch k {
		// Transfer these 2
		case "user-agent":
			out = append(out, grpc.WithUserAgent(v[0]))
		case "host":
			out = append(out, grpc.WithAuthority(v[0]))
		default:
			outMD[k] = v
		}
	}
	log.Infof("Extracted dial options: %+v", out)
	return out, outMD
}
