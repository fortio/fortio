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
	"os"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"istio.io/fortio/fnet"
	"istio.io/fortio/log"
	"istio.io/fortio/stats"
)

const (
	// DefaultHealthServiceName is the default health service name used by fortio.
	DefaultHealthServiceName = "ping"
)

type pingSrv struct {
}

func (s *pingSrv) Ping(c context.Context, in *PingMessage) (*PingMessage, error) {
	log.LogVf("Ping called %+v (ctx %+v)", *in, c)
	out := *in
	out.Ts = time.Now().UnixNano()
	return &out, nil
}

// PingServer starts a grpc ping (and health) echo server.
// returns the port being bound (useful when passing "0" as the port to
// get a dynamic server). Pass the healthServiceName to use for the
// grpc service name health check (or pass DefaultHealthServiceName)
// to be marked as SERVING.
func PingServer(port string, healthServiceName string) int {
	socket, addr := fnet.Listen("grpc '"+healthServiceName+"'", port)
	grpcServer := grpc.NewServer()
	reflection.Register(grpcServer)
	healthServer := health.NewServer()
	healthServer.SetServingStatus(healthServiceName, grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)
	RegisterPingServerServer(grpcServer, &pingSrv{})
	go func() {
		if err := grpcServer.Serve(socket); err != nil {
			log.Fatalf("failed to start grpc server: %v", err)
		}
	}()
	return addr.Port
}

// PingClientCall calls the ping service (presumably running as PingServer on
// the destination).
func PingClientCall(serverAddr string, tls bool, n int, payload string) (float64, error) {
	conn, err := Dial(serverAddr, tls) // somehow this never seem to error out, error comes later
	if err != nil {
		return -1, err // error already logged
	}
	msg := &PingMessage{Payload: payload}
	cli := NewPingServerClient(conn)
	// Warm up:
	_, err = cli.Ping(context.Background(), msg)
	if err != nil {
		log.Errf("grpc error from Ping0 %v", err)
		return -1, err
	}
	skewHistogram := stats.NewHistogram(-10, 2)
	rttHistogram := stats.NewHistogram(0, 10)
	for i := 1; i <= n; i++ {
		msg.Seq = int64(i)
		t1a := time.Now().UnixNano()
		msg.Ts = t1a
		res1, err := cli.Ping(context.Background(), msg)
		t2a := time.Now().UnixNano()
		if err != nil {
			log.Errf("grpc error from Ping1 iter %d: %v", i, err)
			return -1, err
		}
		t1b := res1.Ts
		res2, err := cli.Ping(context.Background(), msg)
		t3a := time.Now().UnixNano()
		t2b := res2.Ts
		if err != nil {
			log.Errf("grpc error from Ping2 iter %d: %v", i, err)
			return -1, err
		}
		rt1 := t2a - t1a
		rttHistogram.Record(float64(rt1) / 1000.)
		rt2 := t3a - t2a
		rttHistogram.Record(float64(rt2) / 1000.)
		rtR := t2b - t1b
		rttHistogram.Record(float64(rtR) / 1000.)
		midR := t1b + (rtR / 2)
		avgRtt := (rt1 + rt2 + rtR) / 3
		x := (midR - t2a)
		log.Infof("Ping RTT %d (avg of %d, %d, %d ns) clock skew %d",
			avgRtt, rt1, rtR, rt2, x)
		skewHistogram.Record(float64(x) / 1000.)
		msg = res2
	}
	skewHistogram.Print(os.Stdout, "Clock skew histogram usec", []float64{50})
	rttHistogram.Print(os.Stdout, "RTT histogram usec", []float64{50})
	return rttHistogram.Avg(), nil
}

// HealthResultMap short cut for the map of results to count. -1 for errors.
type HealthResultMap map[grpc_health_v1.HealthCheckResponse_ServingStatus]int64

// GrpcHealthCheck makes a grpc client call to the standard grpc health check
// service.
func GrpcHealthCheck(serverAddr string, tls bool, svcname string, n int) (*HealthResultMap, error) {
	log.Debugf("GrpcHealthCheck for %s tls %v svc '%s', %d iterations", serverAddr, tls, svcname, n)
	conn, err := Dial(serverAddr, tls)
	if err != nil {
		return nil, err
	}
	msg := &grpc_health_v1.HealthCheckRequest{Service: svcname}
	cli := grpc_health_v1.NewHealthClient(conn)
	rttHistogram := stats.NewHistogram(0, 10)
	statuses := make(HealthResultMap)

	for i := 1; i <= n; i++ {
		start := time.Now()
		res, err := cli.Check(context.Background(), msg)
		dur := time.Since(start)
		log.LogVf("Reply from health check %d: %+v", i, res)
		if err != nil {
			log.Errf("grpc error from Check %v", err)
			return nil, err
		}
		statuses[res.Status]++
		rttHistogram.Record(dur.Seconds() * 1000000.)
	}
	rttHistogram.Print(os.Stdout, "RTT histogram usec", []float64{50})
	for k, v := range statuses {
		fmt.Printf("Health %s : %d\n", k.String(), v)
	}
	return &statuses, nil
}
