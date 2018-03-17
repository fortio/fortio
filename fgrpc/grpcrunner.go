// Copyright 2017 Istio Authors
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

package fgrpc // import "istio.io/fortio/fgrpc"

import (
	"context"
	"fmt"
	"net"
	"os"
	"runtime"
	"runtime/pprof"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health/grpc_health_v1"

	"strings"

	"istio.io/fortio/fnet"
	"istio.io/fortio/log"
	"istio.io/fortio/periodic"
)

const (
	// DefaultGRPCPort is the Fortio gRPC server default port number.
	DefaultGRPCPort  = "8079"
	defaultHTTPPort  = "80"
	defaultHTTPSPort = "443"
	prefixHTTP       = "http://"
	prefixHTTPS      = "https://"
)

// Dial dials grpc either using insecure or using default tls setup.
func Dial(serverAddr string, cert string) (conn *grpc.ClientConn, err error) {
	var opts []grpc.DialOption
	var creds credentials.TransportCredentials
	switch {
	case cert != "":
		creds, err = credentials.NewClientTLSFromFile(cert, "")
		if err != nil {
			log.Errf("Invalid TLS credentials: %v\n", err)
			return nil, err
		}
		log.Infof("Using server certificate %v to construct TLS credentials", cert)
		opts = append(opts, grpc.WithTransportCredentials(creds))
	case strings.HasPrefix(serverAddr, "https://"):
		creds = credentials.NewTLS(nil)
		opts = append(opts, grpc.WithTransportCredentials(creds))
	default:
		opts = append(opts, grpc.WithInsecure())
	}
	serverAddr = grpcDestination(serverAddr)
	conn, err = grpc.Dial(serverAddr, opts...)
	if err != nil {
		log.Errf("failed to connect to %s: %v", serverAddr, err)
	}
	return conn, err
}

// TODO: refactor common parts between http and grpc runners

// GRPCRunnerResults is the aggregated result of an GRPCRunner.
// Also is the internal type used per thread/goroutine.
type GRPCRunnerResults struct {
	periodic.RunnerResults
	client      grpc_health_v1.HealthClient
	req         grpc_health_v1.HealthCheckRequest
	RetCodes    HealthResultMap
	Destination string
}

// Run exercises GRPC health check at the target QPS.
// To be set as the Function in RunnerOptions.
func (grpcstate *GRPCRunnerResults) Run(t int) {
	log.Debugf("Calling in %d", t)
	res, err := grpcstate.client.Check(context.Background(), &grpcstate.req)
	log.Debugf("Got %v %v", err, res)
	if err != nil {
		log.Warnf("Error making health check %v", err)
		grpcstate.RetCodes[-1]++
	} else {
		grpcstate.RetCodes[res.Status]++
	}
}

// GRPCRunnerOptions includes the base RunnerOptions plus http specific
// options.
type GRPCRunnerOptions struct {
	periodic.RunnerOptions
	Destination        string
	Service            string
	Profiler           string // file to save profiles to. defaults to no profiling
	AllowInitialErrors bool   // whether initial errors don't cause an abort
	Cert               string // Path to server certificate for secure grpc
}

// RunGRPCTest runs an http test and returns the aggregated stats.
func RunGRPCTest(o *GRPCRunnerOptions) (*GRPCRunnerResults, error) {
	log.Infof("Starting grpc test for %s with %d threads at %.1f qps", o.Destination, o.NumThreads, o.QPS)
	r := periodic.NewPeriodicRunner(&o.RunnerOptions)
	defer r.Options().Abort()
	numThreads := r.Options().NumThreads
	total := GRPCRunnerResults{
		RetCodes:    make(HealthResultMap),
		Destination: o.Destination,
	}
	grpcstate := make([]GRPCRunnerResults, numThreads)
	out := r.Options().Out // Important as the default value is set from nil to stdout inside NewPeriodicRunner
	for i := 0; i < numThreads; i++ {
		r.Options().Runners[i] = &grpcstate[i]
		conn, err := Dial(o.Destination, o.Cert)
		if err != nil {
			log.Errf("Error in grpc dial for %s %v", o.Destination, err)
			return nil, err
		}
		grpcstate[i].client = grpc_health_v1.NewHealthClient(conn)
		if grpcstate[i].client == nil {
			return nil, fmt.Errorf("unable to create client %d for %s", i, o.Destination)
		}
		grpcstate[i].req = grpc_health_v1.HealthCheckRequest{Service: o.Service}
		if o.Exactly <= 0 {
			_, err = grpcstate[i].client.Check(context.Background(), &grpcstate[i].req)
			if !o.AllowInitialErrors && err != nil {
				log.Errf("Error in first grpc health check call for %s %v", o.Destination, err)
				return nil, err
			}
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
		pprof.StartCPUProfile(fc) //nolint: gas,errcheck
	}
	total.RunnerResults = r.Run()
	if o.Profiler != "" {
		pprof.StopCPUProfile()
		fm, err := os.Create(o.Profiler + ".mem")
		if err != nil {
			log.Critf("Unable to create .mem profile: %v", err)
			return nil, err
		}
		runtime.GC()               // get up-to-date statistics
		pprof.WriteHeapProfile(fm) // nolint:gas,errcheck
		fm.Close()                 // nolint:gas,errcheck
		fmt.Printf("Wrote profile data to %s.{cpu|mem}\n", o.Profiler)
	}
	// Numthreads may have reduced
	numThreads = r.Options().NumThreads
	keys := []grpc_health_v1.HealthCheckResponse_ServingStatus{}
	for i := 0; i < numThreads; i++ {
		// Q: is there some copying each time stats[i] is used?
		for k := range grpcstate[i].RetCodes {
			if _, exists := total.RetCodes[k]; !exists {
				keys = append(keys, k)
			}
			total.RetCodes[k] += grpcstate[i].RetCodes[k]
		}
		// TODO: if grpc client needs 'cleanup'/Close like http one, do it on original NumThreads
	}
	// Cleanup state:
	r.Options().ReleaseRunners()
	for _, k := range keys {
		fmt.Fprintf(out, "Health %s : %d\n", k.String(), total.RetCodes[k])
	}
	return &total, nil
}

// grpcDestination parses dest and returns dest:port based on dest being
// a hostname, IP address, hostname:port, or ip:port. The original dest is
// returned if dest is an invalid hostname or invalid IP address. An http/https
// prefix is removed from dest if one exists and the port number is set to
// DefaultHTTPPort for http, DefaultHTTPSPort for https, or DefaultGRPCPort
// if http, https, or :port is not specified in dest.
// TODO: change/fix this (NormalizePort and more)
func grpcDestination(dest string) (parsedDest string) {
	var port string
	// strip any unintentional http/https scheme prefixes from dest
	// and set the port number.
	switch {
	case strings.HasPrefix(dest, prefixHTTP):
		parsedDest = strings.Replace(dest, prefixHTTP, "", 1)
		port = defaultHTTPPort
		log.Infof("stripping http scheme. grpc destination: %v: grpc port: %s",
			parsedDest, port)
	case strings.HasPrefix(dest, prefixHTTPS):
		parsedDest = strings.Replace(dest, prefixHTTPS, "", 1)
		port = defaultHTTPSPort
		log.Infof("stripping https scheme. grpc destination: %v. grpc port: %s",
			parsedDest, port)
	default:
		parsedDest = dest
		port = DefaultGRPCPort
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
