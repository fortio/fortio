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

package periodic

import (
	"sync"
	"testing"
	"time"
)

type Noop struct{}

func (n *Noop) Run(t int) {
}

func TestNewPeriodicRunner(t *testing.T) {
	var tests = []struct {
		qps                float64 // input
		numThreads         int     // input
		expectedQPS        float64 // expected
		expectedNumThreads int     // expected
	}{
		{qps: 0.1, numThreads: 1, expectedQPS: 0.1, expectedNumThreads: 1},
		{qps: 1, numThreads: 3, expectedQPS: 1, expectedNumThreads: 3},
		{qps: 10, numThreads: 10, expectedQPS: 10, expectedNumThreads: 10},
		{qps: 100000, numThreads: 10, expectedQPS: 100000, expectedNumThreads: 10},
		{qps: 0.5, numThreads: 1, expectedQPS: 0.5, expectedNumThreads: 1},
		// Error cases negative qps same as -1 qps == max speed
		{qps: -10, numThreads: 0, expectedQPS: -1, expectedNumThreads: 4},
		// Need at least 1 thread
		{qps: 0, numThreads: -6, expectedQPS: DefaultRunnerOptions.QPS, expectedNumThreads: 1},
	}
	for _, tst := range tests {
		o := RunnerOptions{
			QPS:        tst.qps,
			NumThreads: tst.numThreads,
		}
		r := newPeriodicRunner(&o)
		r.MakeRunners(&Noop{})
		if r.QPS != tst.expectedQPS {
			t.Errorf("qps: got %f, not as expected %f", r.QPS, tst.expectedQPS)
		}
		if r.NumThreads != tst.expectedNumThreads {
			t.Errorf("threads: with %d input got %d, not as expected %d",
				tst.numThreads, r.NumThreads, tst.expectedNumThreads)
		}
	}
}

type TestCount struct {
	count *int64
	lock  *sync.Mutex
}

func (c *TestCount) Run(i int) {
	c.lock.Lock()
	(*c.count)++
	c.lock.Unlock()
	time.Sleep(50 * time.Millisecond)
}

func TestStart(t *testing.T) {
	var count int64
	var lock sync.Mutex
	c := TestCount{&count, &lock}
	o := RunnerOptions{
		QPS:        11.4,
		NumThreads: 1,
		Duration:   1 * time.Second,
	}
	r := NewPeriodicRunner(&o)
	r.Options().MakeRunners(&c)
	count = 0
	r.Run()
	if count != 11 {
		t.Errorf("Test executed unexpected number of times %d instead %d", count, 11)
	}
	count = 0
	oo := r.Options()
	oo.NumThreads = 10 // will be lowered to 5 so 10 calls (2 in each thread)
	r.Run()
	if count != 10 {
		t.Errorf("MT Test executed unexpected number of times %d instead %d", count, 10)
	}
	// note: it's kind of a bug this only works after Run() and not before
	if oo.NumThreads != 5 {
		t.Errorf("Lowering of thread count broken, got %d instead of 5", oo.NumThreads)
	}
	count = 0
	oo.Duration = 1 * time.Nanosecond
	r.Run()
	if count != 2 {
		t.Errorf("Test executed unexpected number of times %d instead minimum 2", count)
	}
}

func TestStartMaxQps(t *testing.T) {
	var count int64
	var lock sync.Mutex
	c := TestCount{&count, &lock}
	o := RunnerOptions{
		QPS:        -1, // max speed (0 is default qps, not max)
		NumThreads: 4,
		Duration:   140 * time.Millisecond,
	}
	r := NewPeriodicRunner(&o)
	r.Options().MakeRunners(&c)
	count = 0
	var res1 HasRunnerResult // test that interface
	res := r.Run()
	res1 = res.Result()
	expected := int64(3 * 4) // can start 3 50ms in 140ms * 4 threads
	// Check the count both from the histogram and from our own test counter:
	actual := res1.Result().DurationHistogram.Count
	if actual != expected {
		t.Errorf("MaxQpsTest executed unexpected number of times %d instead %d", actual, expected)
	}
	if count != expected {
		t.Errorf("MaxQpsTest executed unexpected number of times %d instead %d", count, expected)
	}
}

func TestID(t *testing.T) {
	var tests = []struct {
		labels string // input
		id     string // expected suffix after the date
	}{
		{"", ""},
		{"abcDEF123", "_abcDEF123"},
		{"A!@#$%^&*()-+=/'B", "_A_B"},
		// Ends with non alpha, skip last _
		{"A  ", "_A"},
		{" ", ""},
		// truncated to fit 64 (17 from date/time + _ + 46 from labels)
		{"123456789012345678901234567890123456789012345678901234567890", "_1234567890123456789012345678901234567890123456"},
	}
	startTime := time.Date(2001, time.January, 2, 3, 4, 5, 0, time.Local)
	prefix := "2001-01-02-030405"
	for _, tst := range tests {
		o := RunnerResults{
			StartTime: startTime,
			Labels:    tst.labels,
		}
		id := o.ID()
		expected := prefix + tst.id
		if id != expected {
			t.Errorf("id: got %s, not as expected %s", id, expected)
		}
	}
}

func TestInfiniteDurationAndAbort(t *testing.T) {
	var count int64
	var lock sync.Mutex
	c := TestCount{&count, &lock}
	o := RunnerOptions{
		QPS:        10,
		NumThreads: 1,
		Duration:   -1, // infinite but we'll abort after 1sec
	}
	r := NewPeriodicRunner(&o)
	r.Options().MakeRunners(&c)
	count = 0
	go func() {
		time.Sleep(1 * time.Second)
		r.Options().Abort()
	}()
	r.Run()
	if count != 11 { // aborts during sleep so + 1 - may be brittle?
		t.Errorf("Test executed unexpected number of times %d instead %d", count, 11)
	}
}

func TestSleepFallingBehind(t *testing.T) {
	var count int64
	var lock sync.Mutex
	c := TestCount{&count, &lock}
	o := RunnerOptions{
		QPS:        1000000, // similar to max qps but with sleep falling behind
		NumThreads: 4,
		Duration:   140 * time.Millisecond,
	}
	r := NewPeriodicRunner(&o)
	r.Options().MakeRunners(&c)
	count = 0
	res := r.Run()
	expected := int64(3 * 4) // can start 3 50ms in 140ms * 4 threads
	// Check the count both from the histogram and from our own test counter:
	actual := res.DurationHistogram.Count
	if actual != expected {
		t.Errorf("Extra high qps executed unexpected number of times %d instead %d", actual, expected)
	}
	if count != expected {
		t.Errorf("Extra high qps executed unexpected number of times %d instead %d", count, expected)
	}
}
