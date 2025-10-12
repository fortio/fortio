// Copyright 2023 Fortio Authors
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

// Package metrics provides a minimal metrics export package for Fortio.
package metrics // import "fortio.org/fortio/metrics"

import (
	"io"
	"net/http"
	"runtime"
	"strconv"

	"fortio.org/fortio/rapi"
	"fortio.org/log"
	"fortio.org/scli"
)

// Exporter writes minimal prometheus style metrics to the http.ResponseWriter.
func Exporter(w http.ResponseWriter, r *http.Request) {
	log.LogRequest(r, "metrics")
	w.Header().Set("Content-Type", "text/plain")
	_, _ = io.WriteString(w, `# HELP fortio_num_fd Number of open file descriptors
# TYPE fortio_num_fd gauge
fortio_num_fd `)
	_, _ = io.WriteString(w, strconv.Itoa(scli.NumFD()))
	cur, total := rapi.RunMetrics()
	_, _ = io.WriteString(w, `
# HELP fortio_running Number of currently running load tests
# TYPE fortio_running gauge
fortio_running `)
	_, _ = io.WriteString(w, strconv.Itoa(cur))
	_, _ = io.WriteString(w, `
# HELP fortio_runs_total Number of runs so far
# TYPE fortio_runs_total counter
fortio_runs_total `)
	_, _ = io.WriteString(w, strconv.FormatInt(total, 10))
	_, _ = io.WriteString(w, `
# HELP fortio_goroutines Current number of goroutines
# TYPE fortio_goroutines gauge
fortio_goroutines `)
	_, _ = io.WriteString(w, strconv.FormatInt(int64(runtime.NumGoroutine()), 10))
	_, _ = io.WriteString(w, "\n")
}
