package results

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"

	"istio.io/fortio/log"
	"istio.io/fortio/stats"
	"time"
	"os"
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
//
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

func LoadResultFiles(files[] string) ([]*RunnerResults, error) {
	res := make([]*RunnerResults, 0, len(files))
	for _, file := range files {
		raw, err := ioutil.ReadFile(file)
		if err != nil {
			log.Errf("Error reading file %s", file)
			return nil, err
		}
		result := RunnerResults{}
		json.Unmarshal(raw, &result)
		res = append(res, &result)
	}
	return res, nil
}

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
		mergedResults.DurationHistogram = stats.MergeHistograms(mergedResults.DurationHistogram.Histogram(), result.DurationHistogram.Histogram()).Export()

		// merge RetCodes (needs parsing of results as http/grpc results)
	}

	mergedResults.ID()
	fmt.Printf("merged: %v", mergedResults)
	SaveJSON(mergedResults, "-")
	// return combined results
}

func formatDate(d *time.Time) string {
	return fmt.Sprintf("%d-%02d-%02d-%02d%02d%02d", d.Year(), d.Month(), d.Day(),
		d.Hour(), d.Minute(), d.Second())
}