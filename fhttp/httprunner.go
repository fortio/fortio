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

package fhttp

import (
	"fmt"
	"net/http"
	"os"
	"runtime"
	"runtime/pprof"
	"sort"
	"sync"

	"fortio.org/fortio/log"
	"fortio.org/fortio/periodic"
	"fortio.org/fortio/stats"
)

// Most of the code in this file is the library-fication of code originally
// in cmd/fortio/main.go

// HTTPRunnerResults is the aggregated result of an HTTPRunner.
// Also is the internal type used per thread/goroutine.
type HTTPRunnerResults struct {
	periodic.RunnerResults
	client     Fetcher
	RetCodes   map[int]int64
	IPCountMap map[string]int // TODO: Move it to a shared results struct where all runner should have this field
	// internal type/data
	sizes       *stats.Histogram
	headerSizes *stats.Histogram
	// exported result
	HTTPOptions
	Sizes       *stats.HistogramData
	HeaderSizes *stats.HistogramData
	Sockets     []int64
	SocketCount int64
	// Connection Time stats
	ConnectionStats *stats.HistogramData
	// http code to abort the run on (-1 for connection or other socket error)
	AbortOn int
	aborter *periodic.Aborter
}

// Run tests http request fetching. Main call being run at the target QPS.
// To be set as the Function in RunnerOptions.
func (httpstate *HTTPRunnerResults) Run(t int) (bool, string) {
	log.Debugf("Calling in %d", t)
	code, body, headerSize := httpstate.client.Fetch()
	size := len(body)
	log.Debugf("Got in %3d hsz %d sz %d - will abort on %d", code, headerSize, size, httpstate.AbortOn)
	httpstate.RetCodes[code]++
	httpstate.sizes.Record(float64(size))
	httpstate.headerSizes.Record(float64(headerSize))
	if httpstate.AbortOn == code {
		httpstate.aborter.Abort(false)
		log.Infof("Aborted run because of code %d - data %s", code, DebugSummary(body, 1024))
	}
	if code == http.StatusOK {
		return true, "200"
	}
	return false, fmt.Sprint(code)
}

// HTTPRunnerOptions includes the base RunnerOptions plus http specific
// options.
type HTTPRunnerOptions struct {
	periodic.RunnerOptions
	HTTPOptions               // Need to call Init() to initialize
	Profiler           string // file to save profiles to. defaults to no profiling
	AllowInitialErrors bool   // whether initial errors don't cause an abort
	// Which status code cause an abort of the run (default 0 = don't abort; reminder -1 is returned for socket errors)
	AbortOn int
}

// RunHTTPTest runs an http test and returns the aggregated stats.
//
//nolint:funlen, gocognit, gocyclo, maintidx
func RunHTTPTest(o *HTTPRunnerOptions) (*HTTPRunnerResults, error) {
	o.RunType = "HTTP"
	warmupMode := "parallel"
	if o.SequentialWarmup {
		warmupMode = "sequential"
	}

	connReuseMsg := ""
	if o.ConnReuseRange != [2]int{0, 0} {
		connReuseMsg = fmt.Sprintf(", with connection reuse [%d, %d]", o.ConnReuseRange[0], o.ConnReuseRange[1])
	}
	log.Infof("Starting http test for %s with %d threads at %.1f qps and %s warmup%s",
		o.URL, o.NumThreads, o.QPS, warmupMode, connReuseMsg)
	r := periodic.NewPeriodicRunner(&o.RunnerOptions)
	if o.HTTPOptions.Resolution <= 0 {
		// Set both connect histogram params when Resolution isn't set explicitly on the HTTP options
		// (that way you can set the offet to 0 in connect and to something else for the call)
		o.HTTPOptions.Resolution = r.Options().Resolution
		o.HTTPOptions.Offset = r.Options().Offset
	}
	defer r.Options().Abort()
	numThreads := r.Options().NumThreads // can change during run for c > 2 n
	o.HTTPOptions.Init(o.URL)
	out := r.Options().Out // Important as the default value is set from nil to stdout inside NewPeriodicRunner
	total := HTTPRunnerResults{
		HTTPOptions: o.HTTPOptions,
		RetCodes:    make(map[int]int64),
		IPCountMap:  make(map[string]int),
		sizes:       stats.NewHistogram(0, 100),
		headerSizes: stats.NewHistogram(0, 5),
		AbortOn:     o.AbortOn,
		aborter:     r.Options().Stop,
	}
	httpstate := make([]HTTPRunnerResults, numThreads)
	// First build all the clients sequentially. This ensures we do not have data races when
	// constructing requests.
	for i := 0; i < numThreads; i++ {
		r.Options().Runners[i] = &httpstate[i]
		// Temp mutate the option so each client gets a logging id
		o.HTTPOptions.ID = i
		// Create a client (and transport) and connect once for each 'thread'
		var err error
		httpstate[i].client, err = NewClient(&o.HTTPOptions)
		// nil check on interface doesn't work
		if err != nil {
			return nil, err
		}
		if o.SequentialWarmup && o.Exactly <= 0 {
			code, data, headerSize := httpstate[i].client.Fetch()
			if !o.AllowInitialErrors && !codeIsOK(code) {
				return nil, fmt.Errorf("error %d for %s: %q", code, o.URL, string(data))
			}
			if i == 0 && log.LogVerbose() {
				log.LogVf("first hit of url %s: status %03d, headers %d, total %d\n%s\n", o.URL, code, headerSize, len(data), data)
			}
		}
		// Setup the stats for each 'thread'
		httpstate[i].sizes = total.sizes.Clone()
		httpstate[i].headerSizes = total.headerSizes.Clone()
		httpstate[i].RetCodes = make(map[int]int64)
		httpstate[i].AbortOn = total.AbortOn
		httpstate[i].aborter = total.aborter
	}
	if o.Exactly <= 0 && !o.SequentialWarmup {
		warmup := errgroup{}
		for i := 0; i < numThreads; i++ {
			i := i
			warmup.Go(func() error {
				code, data, headerSize := httpstate[i].client.Fetch()
				if !o.AllowInitialErrors && !codeIsOK(code) {
					return fmt.Errorf("error %d for %s: %q", code, o.URL, string(data))
				}
				if i == 0 && log.LogVerbose() {
					log.LogVf("first hit of url %s: status %03d, headers %d, total %d\n%s\n", o.URL, code, headerSize, len(data), data)
				}
				return nil
			})
		}
		if err := warmup.Wait(); err != nil {
			return nil, err
		}
	}
	// TODO avoid copy pasta with grpcrunner
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
		_, _ = fmt.Fprintf(out, "Wrote profile data to %s.{cpu|mem}\n", o.Profiler)
	}
	// Connection stats, aggregated
	connectionStats := stats.NewHistogram(o.HTTPOptions.Offset.Seconds(), o.HTTPOptions.Resolution)
	// Numthreads may have reduced:
	numThreads = total.RunnerResults.NumThreads
	// But we also must cleanup all the created clients.
	keys := []int{}
	fmt.Fprintf(out, "# Socket and IP used for each connection:\n")
	for i := 0; i < numThreads; i++ {
		// Get the report on the IP address each thread use to send traffic
		occurrence, connStats := httpstate[i].client.GetIPAddress()
		currentSocketUsed := connStats.Count
		httpstate[i].client.Close()
		fmt.Fprintf(out, "[%d] %3d socket used, resolved to %s ", i, currentSocketUsed, occurrence.PrintAndAggregate(total.IPCountMap))
		connStats.Counter.Print(out, "connection timing")
		total.SocketCount += currentSocketUsed
		total.Sockets = append(total.Sockets, currentSocketUsed)
		// Q: is there some copying each time stats[i] is used?
		for k := range httpstate[i].RetCodes {
			if _, exists := total.RetCodes[k]; !exists {
				keys = append(keys, k)
			}
			total.RetCodes[k] += httpstate[i].RetCodes[k]
		}
		total.sizes.Transfer(httpstate[i].sizes)
		total.headerSizes.Transfer(httpstate[i].headerSizes)
		connectionStats.Transfer(connStats)
	}
	total.ConnectionStats = connectionStats.Export().CalcPercentiles(o.Percentiles)
	if log.Log(log.Info) {
		total.ConnectionStats.Print(out, "Connection time histogram (s)")
	} else if log.Log(log.Warning) {
		connectionStats.Counter.Print(out, "Connection time (s)")
	}

	// Sort the ip address form largest to smallest based on its usage count
	ipList := make([]string, 0, len(total.IPCountMap))
	for k := range total.IPCountMap {
		ipList = append(ipList, k)
	}

	sort.Slice(ipList, func(i, j int) bool {
		return total.IPCountMap[ipList[i]] > total.IPCountMap[ipList[j]]
	})

	// Cleanup state: (original num thread)
	r.Options().ReleaseRunners()
	sort.Ints(keys)
	totalCount := float64(total.DurationHistogram.Count)
	_, _ = fmt.Fprintf(out, "Sockets used: %d (for perfect keepalive, would be %d)\n", total.SocketCount, r.Options().NumThreads)
	_, _ = fmt.Fprintf(out, "Uniform: %t, Jitter: %t\n", total.Uniform, total.Jitter)
	_, _ = fmt.Fprintf(out, "IP addresses distribution:\n")
	for _, v := range ipList {
		_, _ = fmt.Fprintf(out, "%s: %d\n", v, total.IPCountMap[v])
	}
	for _, k := range keys {
		_, _ = fmt.Fprintf(out, "Code %3d : %d (%.1f %%)\n", k, total.RetCodes[k], 100.*float64(total.RetCodes[k])/totalCount)
	}
	total.HeaderSizes = total.headerSizes.Export()
	total.Sizes = total.sizes.Export()
	if log.LogVerbose() {
		total.HeaderSizes.Print(out, "Response Header Sizes Histogram")
		total.Sizes.Print(out, "Response Body/Total Sizes Histogram")
	} else if log.Log(log.Warning) {
		total.headerSizes.Counter.Print(out, "Response Header Sizes")
		total.sizes.Counter.Print(out, "Response Body/Total Sizes")
	}
	return &total, nil
}

// An errgroup is a collection of goroutines working on subtasks that are part of
// the same overall task.
type errgroup struct {
	wg sync.WaitGroup

	errOnce sync.Once
	err     error
}

// Wait blocks until all function calls from the Go method have returned, then
// returns the first non-nil error (if any) from them.
func (g *errgroup) Wait() error {
	g.wg.Wait()
	return g.err
}

// Go calls the given function in a new goroutine.
//
// The first call to return a non-nil error cancels the group; its error will be
// returned by Wait.
func (g *errgroup) Go(f func() error) {
	g.wg.Add(1)

	go func() {
		defer g.wg.Done()

		if err := f(); err != nil {
			g.errOnce.Do(func() {
				g.err = err
			})
		}
	}()
}
