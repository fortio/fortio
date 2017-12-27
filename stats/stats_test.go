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

package stats

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"istio.io/fortio/log"
)

func TestCounter(t *testing.T) {
	c := NewHistogram(22, 0.1)
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	c.Counter.Print(w, "test1c")
	expected := "test1c : count 0 avg NaN +/- NaN min 0 max 0 sum 0\n"
	c.Print(w, "test1h", []float64{50.0})
	expected += "test1h : no data\n"
	c.Record(23.1)
	c.Counter.Print(w, "test2")
	expected += "test2 : count 1 avg 23.1 +/- 0 min 23.1 max 23.1 sum 23.1\n"
	c.Record(22.9)
	c.Counter.Print(w, "test3")
	expected += "test3 : count 2 avg 23 +/- 0.1 min 22.9 max 23.1 sum 46\n"
	c.Record(23.1)
	c.Record(22.9)
	c.Counter.Print(w, "test4")
	expected += "test4 : count 4 avg 23 +/- 0.1 min 22.9 max 23.1 sum 92\n"
	c.Record(1023)
	c.Record(-977)
	c.Counter.Print(w, "test5")
	// note that stddev of 577.4 below is... whatever the code said
	finalExpected := " : count 6 avg 23 +/- 577.4 min -977 max 1023 sum 138\n"
	expected += "test5" + finalExpected
	// Try the Log() function too:
	log.SetOutput(w)
	log.SetFlags(0)
	*log.LogFileAndLine = false
	*log.LogPrefix = ""
	c.Counter.Log("testLogC")
	expected += "I testLogC" + finalExpected
	w.Flush() // nolint: errcheck
	actual := b.String()
	if actual != expected {
		t.Errorf("unexpected1:\n%s\nvs:\n%s\n", actual, expected)
	}
	b.Reset()
	c.Log("testLogH", nil)
	w.Flush() // nolint: errcheck
	actual = b.String()
	expected = "I testLogH" + finalExpected + `# range, mid point, percentile, count
>= -977 < 22.1 , -477.45 , 16.67, 1
>= 22.8 < 22.9 , 22.85 , 50.00, 2
>= 23.1 < 23.2 , 23.15 , 83.33, 2
>= 1022 <= 1023 , 1022.5 , 100.00, 1
`
	if actual != expected {
		t.Errorf("unexpected2:\n%s\nvs:\n%s\n", actual, expected)
	}
}

func TestTransferCounter(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	var c1 Counter
	c1.Record(10)
	c1.Record(20)
	var c2 Counter
	c2.Record(80)
	c2.Record(90)
	c1a := c1
	c2a := c2
	var c3 Counter
	c1.Print(w, "c1 before merge")
	c2.Print(w, "c2 before merge")
	c1.Transfer(&c2)
	c1.Print(w, "mergedC1C2")
	c2.Print(w, "c2 after merge")
	// reverse (exercise min if)
	c2a.Transfer(&c1a)
	c2a.Print(w, "mergedC2C1")
	// test transfer into empty - min should be set
	c3.Transfer(&c1)
	c1.Print(w, "c1 should now be empty")
	c3.Print(w, "c3 after merge - 1")
	// test empty transfer - shouldn't reset min/no-op
	c3.Transfer(&c2)
	c3.Print(w, "c3 after merge - 2")
	w.Flush() // nolint: errcheck
	actual := b.String()
	expected := `c1 before merge : count 2 avg 15 +/- 5 min 10 max 20 sum 30
c2 before merge : count 2 avg 85 +/- 5 min 80 max 90 sum 170
mergedC1C2 : count 4 avg 50 +/- 35.36 min 10 max 90 sum 200
c2 after merge : count 0 avg NaN +/- NaN min 0 max 0 sum 0
mergedC2C1 : count 4 avg 50 +/- 35.36 min 10 max 90 sum 200
c1 should now be empty : count 0 avg NaN +/- NaN min 0 max 0 sum 0
c3 after merge - 1 : count 4 avg 50 +/- 35.36 min 10 max 90 sum 200
c3 after merge - 2 : count 4 avg 50 +/- 35.36 min 10 max 90 sum 200
`
	if actual != expected {
		t.Errorf("unexpected:\n%s\tvs:\n%s", actual, expected)
	}
}

func TestHistogram(t *testing.T) {
	h := NewHistogram(0, 10)
	h.Record(1)
	h.Record(251)
	h.Record(501)
	h.Record(751)
	h.Record(1001)
	h.Print(os.Stdout, "testHistogram1", []float64{50})
	for i := 25; i <= 100; i += 25 {
		fmt.Printf("%d%% at %g\n", i, h.CalcPercentile(float64(i)))
	}
	var tests = []struct {
		actual   float64
		expected float64
		msg      string
	}{
		{h.Avg(), 501, "avg"},
		{h.CalcPercentile(-1), 1, "p-1"}, // not valid but should return min
		{h.CalcPercentile(0), 1, "p0"},
		{h.CalcPercentile(0.1), 1.045, "p0.1"},
		{h.CalcPercentile(1), 1.45, "p1"},
		{h.CalcPercentile(20), 10, "p20"},         // 20% = first point, 1st bucket is 10
		{h.CalcPercentile(20.1), 250.25, "p20.1"}, // near beginning of bucket of 2nd pt
		{h.CalcPercentile(50), 550, "p50"},
		{h.CalcPercentile(75), 775, "p75"},
		{h.CalcPercentile(90), 1000.5, "p90"},
		{h.CalcPercentile(99), 1000.95, "p99"},
		{h.CalcPercentile(99.9), 1000.995, "p99.9"},
		{h.CalcPercentile(100), 1001, "p100"},
		{h.CalcPercentile(101), 1001, "p101"},
	}
	for _, tst := range tests {
		if tst.actual != tst.expected {
			t.Errorf("%s: got %g, not as expected %g", tst.msg, tst.actual, tst.expected)
		}
	}
}

// CheckEquals checks if actual == expect and fails the test and logs
// failure (including filename:linenum if they are not equal).
func CheckEquals(t *testing.T, actual interface{}, expected interface{}, msg interface{}) {
	if expected != actual {
		_, file, line, _ := runtime.Caller(1)
		file = file[strings.LastIndex(file, "/")+1:]
		fmt.Printf("%s:%d mismatch!\nactual:\n%+v\nexpected:\n%+v\nfor %+v\n", file, line, actual, expected, msg)
		t.Fail()
	}
}

func Assert(t *testing.T, cond bool, msg interface{}) {
	if !cond {
		_, file, line, _ := runtime.Caller(1)
		file = file[strings.LastIndex(file, "/")+1:]
		fmt.Printf("%s:%d assert failure: %+v\n", file, line, msg)
		t.Fail()
	}
}

// Checks properties that should be true for all non empty histograms
func CheckGenericHistogramDataProperties(t *testing.T, e *HistogramData) {
	n := len(e.Data)
	if n <= 0 {
		t.Error("Unexpected empty histogram")
		return
	}
	CheckEquals(t, e.Data[0].Start, e.Min, "first bucket starts at min")
	CheckEquals(t, e.Data[n-1].End, e.Max, "end of last bucket is max")
	CheckEquals(t, e.Data[n-1].Percent, 100., "last bucket is 100%")
	// All buckets in order
	var prev Bucket
	var sum int64
	for i := 0; i < n; i++ {
		b := e.Data[i]
		Assert(t, b.Start <= b.End, "End should always be after Start")
		Assert(t, b.Count > 0, "Every exported bucket should have data")
		Assert(t, b.Percent > 0, "Percentage should always be positive")
		sum += b.Count
		if i > 0 {
			Assert(t, b.Start >= prev.End, "Start of next bucket >= end of previous")
			Assert(t, b.Percent > prev.Percent, "Percentage should be ever increasing")
		}
		prev = b
	}
	CheckEquals(t, sum, e.Count, "Sum in buckets should add up to Counter's count")
}

func TestHistogramExport1(t *testing.T) {
	h := NewHistogram(0, 10)
	e := h.Export(nil) // no crash or error for empty ones
	CheckEquals(t, e.Count, int64(0), "empty is 0 count")
	CheckEquals(t, len(e.Data), 0, "empty is no bucket data")
	h.Record(-137.4)
	h.Record(251)
	h.Record(501)
	h.Record(751)
	h.Record(1001.67)
	e = h.Export([]float64{50, 99, 99.9})
	CheckEquals(t, e.Count, int64(5), "count")
	CheckEquals(t, e.Min, -137.4, "min")
	CheckEquals(t, e.Max, 1001.67, "max")
	n := len(e.Data)
	CheckEquals(t, n, 5, "number of buckets")
	CheckGenericHistogramDataProperties(t, e)
	data, err := json.MarshalIndent(e, "", " ")
	if err != nil {
		t.Error(err)
	}
	CheckEquals(t, string(data), `{
 "Count": 5,
 "Min": -137.4,
 "Max": 1001.67,
 "Sum": 2367.27,
 "Avg": 473.454,
 "StdDev": 394.8242896074151,
 "Data": [
  {
   "Start": -137.4,
   "End": 10,
   "Percent": 20,
   "Count": 1
  },
  {
   "Start": 250,
   "End": 300,
   "Percent": 40,
   "Count": 1
  },
  {
   "Start": 500,
   "End": 600,
   "Percent": 60,
   "Count": 1
  },
  {
   "Start": 700,
   "End": 800,
   "Percent": 80,
   "Count": 1
  },
  {
   "Start": 1000,
   "End": 1001.67,
   "Percent": 100,
   "Count": 1
  }
 ],
 "Percentiles": [
  {
   "Percentile": 50,
   "Value": 550
  },
  {
   "Percentile": 99,
   "Value": 1001.5865
  },
  {
   "Percentile": 99.9,
   "Value": 1001.66165
  }
 ]
}`, "Json output")
}

const (
	NumRandomHistogram = 2000
)

func TestHistogramExportRandom(t *testing.T) {
	for i := 0; i < NumRandomHistogram; i++ {
		// offset [-500,500[  divisor ]0,100]
		offset := (rand.Float64() - 0.5) * 1000
		div := 100 * (1 - rand.Float64())
		numEntries := 1 + rand.Int31n(10000)
		//fmt.Printf("new histogram with offset %g, div %g - will insert %d entries\n", offset, div, numEntries)
		h := NewHistogram(offset, div)
		var n int32
		var min float64
		var max float64
		for ; n < numEntries; n++ {
			v := 3000 * (rand.Float64() - 0.25)
			if n == 0 {
				min = v
				max = v
			} else {
				if v < min {
					min = v
				} else if v > max {
					max = v
				}
			}
			h.Record(v)
		}
		e := h.Export([]float64{0, 50, 100})
		CheckGenericHistogramDataProperties(t, e)
		CheckEquals(t, h.Count, int64(numEntries), "num entries should match")
		CheckEquals(t, h.Min, min, "Min should match")
		CheckEquals(t, h.Max, max, "Max should match")
		CheckEquals(t, e.Percentiles[0].Value, min, "p0 should be min")
		CheckEquals(t, e.Percentiles[2].Value, max, "p100 should be max")
	}
}

func TestHistogramLastBucket(t *testing.T) {
	// Use -1 offset so first bucket is negative values
	h := NewHistogram( /* offset */ -1 /*scale */, 1)
	h.Record(-1)
	h.Record(0)
	h.Record(1)
	h.Record(3)
	h.Record(10)
	h.Record(99998)
	h.Record(99999) // first value of last bucket 100k-offset
	h.Record(200000)
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	h.Print(w, "testLastBucket", []float64{90})
	w.Flush() // nolint: errcheck
	actual := b.String()
	// stdev part is not verified/could be brittle
	expected := `testLastBucket : count 8 avg 50001.25 +/- 7.071e+04 min -1 max 200000 sum 400010
# range, mid point, percentile, count
>= -1 < 0 , -0.5 , 12.50, 1
>= 0 < 1 , 0.5 , 25.00, 1
>= 1 < 2 , 1.5 , 37.50, 1
>= 3 < 4 , 3.5 , 50.00, 1
>= 10 < 11 , 10.5 , 62.50, 1
>= 74999 < 99999 , 87499 , 75.00, 1
>= 99999 <= 200000 , 150000 , 100.00, 2
# target 90% 160000
`
	if actual != expected {
		t.Errorf("unexpected:\n%s\tvs:\n%s", actual, expected)
	}
}

func TestHistogramNegativeNumbers(t *testing.T) {
	h := NewHistogram( /* offset */ -10 /*scale */, 1)
	h.Record(-10)
	h.Record(10)
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	// TODO: fix the p51 (and p1...), should be 0 not 10
	h.Print(w, "testHistogramWithNegativeNumbers", []float64{51})
	w.Flush() // nolint: errcheck
	actual := b.String()
	// stdev part is not verified/could be brittle
	expected := `testHistogramWithNegativeNumbers : count 2 avg 0 +/- 10 min -10 max 10 sum 0
# range, mid point, percentile, count
>= -10 < -9 , -9.5 , 50.00, 1
>= 10 <= 10 , 10 , 100.00, 1
# target 51% 10
`
	if actual != expected {
		t.Errorf("unexpected:\n%s\tvs:\n%s", actual, expected)
	}
}

func TestTransferHistogramWithDifferentScales(t *testing.T) {
	tP := []float64{75.}
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	h1 := NewHistogram(2, 15)
	h1.Record(30)
	h1.Record(40)
	h1.Record(50)
	h2 := NewHistogram(0, 10)
	h2.Record(20)
	h2.Record(90)
	h1.Print(w, "h1 before merge", tP)
	h2.Print(w, "h2 before merge", tP)
	h1.Transfer(h2)
	h1.Print(w, "merged h2 -> h1", tP)
	h2.Print(w, "h2 should now be empty", tP)
	w.Flush()
	actual := b.String()
	expected := `h1 before merge : count 3 avg 40 +/- 8.165 min 30 max 50 sum 120
# range, mid point, percentile, count
>= 30 < 32 , 31 , 33.33, 1
>= 32 < 47 , 39.5 , 66.67, 1
>= 47 <= 50 , 48.5 , 100.00, 1
# target 75% 47.75
h2 before merge : count 2 avg 55 +/- 35 min 20 max 90 sum 110
# range, mid point, percentile, count
>= 20 < 30 , 25 , 50.00, 1
>= 90 <= 90 , 90 , 100.00, 1
# target 75% 90
merged h2 -> h1 : count 5 avg 46 +/- 24.17 min 20 max 90 sum 230
# range, mid point, percentile, count
>= 20 < 32 , 26 , 40.00, 2
>= 32 < 47 , 39.5 , 60.00, 1
>= 47 < 62 , 54.5 , 80.00, 1
>= 77 <= 90 , 83.5 , 100.00, 1
# target 75% 58.25
h2 should now be empty : no data
`
	if actual != expected {
		t.Errorf("unexpected:\n%s\tvs:\n%s", actual, expected)
	}
}

func TestTransferHistogram(t *testing.T) {
	tP := []float64{100.} // TODO: use 75 and fix bug
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	h1 := NewHistogram(0, 10)
	h1.Record(10)
	h1.Record(20)
	h2 := NewHistogram(0, 10)
	h2.Record(80)
	h2.Record(90)
	h1a := h1.Clone()
	h1a.Record(50) // add extra pt to make sure h1a and h1 are distinct
	h2a := h2.Clone()
	h3 := NewHistogram(0, 10)
	h1.Print(w, "h1 before merge", tP)
	h2.Print(w, "h2 before merge", tP)
	h1.Transfer(h2)
	h1.Print(w, "merged h2 -> h1", tP)
	h2.Print(w, "h2 after merge", tP)
	// reverse (exercise min if)
	h2a.Transfer(h1a)
	h2a.Print(w, "merged h1a -> h2a", tP)
	// test transfer into empty - min should be set
	h3.Transfer(h1)
	h1.Print(w, "h1 should now be empty", tP)
	h3.Print(w, "h3 after merge - 1", tP)
	// test empty transfer - shouldn't reset min/no-op
	h3.Transfer(h2)
	h3.Print(w, "h3 after merge - 2", tP)
	w.Flush() // nolint: errcheck
	actual := b.String()
	expected := `h1 before merge : count 2 avg 15 +/- 5 min 10 max 20 sum 30
# range, mid point, percentile, count
>= 10 < 20 , 15 , 50.00, 1
>= 20 <= 20 , 20 , 100.00, 1
# target 100% 20
h2 before merge : count 2 avg 85 +/- 5 min 80 max 90 sum 170
# range, mid point, percentile, count
>= 80 < 90 , 85 , 50.00, 1
>= 90 <= 90 , 90 , 100.00, 1
# target 100% 90
merged h2 -> h1 : count 4 avg 50 +/- 35.36 min 10 max 90 sum 200
# range, mid point, percentile, count
>= 10 < 20 , 15 , 25.00, 1
>= 20 < 30 , 25 , 50.00, 1
>= 80 < 90 , 85 , 75.00, 1
>= 90 <= 90 , 90 , 100.00, 1
# target 100% 90
h2 after merge : no data
merged h1a -> h2a : count 5 avg 50 +/- 31.62 min 10 max 90 sum 250
# range, mid point, percentile, count
>= 10 < 20 , 15 , 20.00, 1
>= 20 < 30 , 25 , 40.00, 1
>= 50 < 60 , 55 , 60.00, 1
>= 80 < 90 , 85 , 80.00, 1
>= 90 <= 90 , 90 , 100.00, 1
# target 100% 90
h1 should now be empty : no data
h3 after merge - 1 : count 4 avg 50 +/- 35.36 min 10 max 90 sum 200
# range, mid point, percentile, count
>= 10 < 20 , 15 , 25.00, 1
>= 20 < 30 , 25 , 50.00, 1
>= 80 < 90 , 85 , 75.00, 1
>= 90 <= 90 , 90 , 100.00, 1
# target 100% 90
h3 after merge - 2 : count 4 avg 50 +/- 35.36 min 10 max 90 sum 200
# range, mid point, percentile, count
>= 10 < 20 , 15 , 25.00, 1
>= 20 < 30 , 25 , 50.00, 1
>= 80 < 90 , 85 , 75.00, 1
>= 90 <= 90 , 90 , 100.00, 1
# target 100% 90
`
	if actual != expected {
		t.Errorf("unexpected:\n%s\tvs:\n%s", actual, expected)
	}
}

func TestParsePercentiles(t *testing.T) {
	var tests = []struct {
		str  string    // input
		list []float64 // expected
		err  bool
	}{
		// Good cases
		{str: "99.9", list: []float64{99.9}},
		{str: "1,2,3", list: []float64{1, 2, 3}},
		{str: "   17, -5.3,  78  ", list: []float64{17, -5.3, 78}},
		// Errors
		{str: "", list: []float64{}, err: true},
		{str: "   ", list: []float64{}, err: true},
		{str: "23,a,46", list: []float64{23}, err: true},
	}
	log.SetLogLevel(log.Debug) // for coverage
	for _, tst := range tests {
		actual, err := ParsePercentiles(tst.str)
		if !reflect.DeepEqual(actual, tst.list) {
			t.Errorf("ParsePercentiles got %#v expected %#v", actual, tst.list)
		}
		if (err != nil) != tst.err {
			t.Errorf("ParsePercentiles got %v error while expecting err:%v for %s",
				err, tst.err, tst.str)
		}
	}
}

func TestRound(t *testing.T) {
	var tests = []struct {
		input    float64
		expected float64
	}{
		{100.00001, 100},
		{100.0001, 100.0001},
		{99.9999999999, 100},
		{100, 100},
		{0.1234499, 0.1234},
		{0.1234567, 0.1235},
		{-0.0000049, 0},
		{-0.499999, -0.5},
		{-0.500001, -0.5},
		{-0.999999, -1},
		{-0.123449, -0.1234},
		{-0.123450, -0.1234}, // should be -0.1235 but we don't deal with <0 specially
	}
	for _, tst := range tests {
		if actual := Round(tst.input); actual != tst.expected {
			t.Errorf("Got %f, expected %f for Round(%.10f)", actual, tst.expected, tst.input)
		}
	}
}
