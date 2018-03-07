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

// Package periodic for fortio (from greek for load) is a set of utilities to
// run a given task at a target rate (qps) and gather statistics - for instance
// http requests.
//
// The main executable using the library is fortio but there
// is also ../histogram to use the stats from the command line and ../echosrv
// as a very light http server that can be used to test proxies etc like
// the Istio components.
package periodic // import "istio.io/fortio/periodic"

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"

	"os"
	"time"

	"istio.io/fortio/log"
	"istio.io/fortio/stats"
)

// RunnerResults encapsulates the actual QPS observed and duration histogram.
type RunnerResults struct {
	Labels            string
	StartTime         time.Time
	RequestedQPS      string
	RequestedDuration string // String version of the requested duration or exact count
	ActualQPS         float64
	ActualDuration    time.Duration
	NumThreads        int
	Version           string
	DurationHistogram *stats.HistogramData
	Exactly           int64 // Echo back the requested count
}

// ID Returns an id for the result: 64 bytes YYYY-MM-DD-HHmmSS_{alpha_labels}
// where alpha_labels is the filtered labels with only alphanumeric characters
// and all non alpha num replaced by _; truncated to 64 bytes.
func (r *RunnerResults) ID() string {
	base := formatDate(&r.StartTime)
	if r.Labels == "" {
		return base
	}
	last := '_'
	base += string(last)
	for _, rune := range r.Labels {
		if (rune >= 'a' && rune <= 'z') || (rune >= 'A' && rune <= 'Z') || (rune >= '0' && rune <= '9') {
			last = rune
		} else {
			if last == '_' {
				continue // only 1 _ separator at a time
			}
			last = '_'
		}
		base += string(last)
	}
	if last == '_' {
		base = base[:len(base)-1]
	}
	if len(base) > 64 {
		return base[:64]
	}
	return base
}

// HasRunnerResult is the interface implictly implemented by HTTPRunnerResults
// and GrpcRunnerResults so the common results can ge extracted irrespective
// of the type.
type HasRunnerResult interface {
	Result() *RunnerResults
}

// Result returns the common RunnerResults.
func (r *RunnerResults) Result() *RunnerResults {
	return r
}

// SaveJSON saves the result as json to the provided file
func SaveJSON(res HasRunnerResult, fileName string) (int, error) {
	var j []byte
	j, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		log.Errf("Unable to json serialize result: %v", err)
		return -1, err
	}
	var f *os.File
	if fileName == "-" {
		f = os.Stdout
		fileName = "stdout"
	} else {
		f, err = os.Create(fileName)
		if err != nil {
			log.Errf("Unable to create %s: %v", fileName, err)
			return -1, err
		}
	}
	n, err := f.Write(append(j, '\n'))
	if err != nil {
		log.Errf("Unable to write json to %s: %v", fileName, err)
		return -1, err
	}
	if f != os.Stdout {
		err := f.Close()
		if err != nil {
			log.Errf("Close error for %s: %v", fileName, err)
			return -1, err
		}
	}
	return n, nil
}

// ParseArgsForResultFiles accepts a list of string and filters it on
// the suffix `.json` and returns the result
func ParseArgsForResultFiles(args []string) ([]string, error) {
	res := make([]string, 0, len(args))
	for _, file := range args {
		file = strings.TrimSpace(file)
		if len(file) == 0 {
			continue
		}
		if strings.HasSuffix(file, ".json") {
			res = append(res, file)
		}
	}
	log.LogVf("Identified %v as result files", res)
	return res, nil
}

// LoadResultFiles accepts a list of filepaths and returns
// marshaled list of RunnerResults
func LoadResultFiles(files []string) ([]*RunnerResults, error) {
	res := make([]*RunnerResults, 0, len(files))
	for _, file := range files {
		raw, err := ioutil.ReadFile(file)
		if err != nil {
			log.Errf("Error reading file %s: %v", file, err)
			return nil, err
		}
		result := RunnerResults{}
		err = json.Unmarshal(raw, &result)
		if err != nil {
			log.Errf("Error unmarshaling to Json %s: %v", file, err)
			return nil, err
		}
		res = append(res, &result)
	}
	return res, nil
}

// MergeRunnerResults accepts a list of RunnerResults and
// combines the data structure to yield a merged result
// typically this should be used for merging results from
// concurrent runs
func MergeRunnerResults(results []*RunnerResults) {
	// TODO determine if results are mergeable
	// TODO specify type of results in the json file metadata
	// TODO check for len(files) > 2 and print usage etc

	// iterate and merge each sequentially
	mergedResults := results[0]
	for _, result := range results[1:] {
		// merge Labels
		// merge StartTime (earliest wins)
		if mergedResults.StartTime.After(result.StartTime) {
			mergedResults.StartTime = result.StartTime
		}
		// merge RequestedQPS
		// merge RequestedDuration
		// ActualQPS
		// ActualDuration
		// merge NumThreads
		mergedResults.NumThreads += result.NumThreads
		// merge Version
		// merge HistogramData
		mergedResults.DurationHistogram = stats.MergeHistograms(
			mergedResults.DurationHistogram.Histogram(),
			result.DurationHistogram.Histogram(),
		).Export()

		// merge RetCodes (needs parsing of results as http/grpc results)
	}

	mergedResults.ID()
	fmt.Printf("merged: %v", mergedResults)
	_, err := SaveJSON(mergedResults, "-")
	if err != nil {
		log.Errf("error during save: %v", err)
	}
	// return combined results
}

func formatDate(d *time.Time) string {
	return fmt.Sprintf("%d-%02d-%02d-%02d%02d%02d", d.Year(), d.Month(), d.Day(),
		d.Hour(), d.Minute(), d.Second())
}
