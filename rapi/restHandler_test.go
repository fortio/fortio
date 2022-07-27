// Copyright 2022 Fortio Authors.
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

package rapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"fortio.org/fortio/fhttp"
)

func TestRestRunner(t *testing.T) {
	mux, addr := fhttp.DynamicHTTPServer(false)
	mux.HandleFunc("/foo/", fhttp.EchoHandler)
	baseURL := fmt.Sprintf("http://localhost:%d/", addr.Port)
	uiPath := "/fortio/"
	AddHandlers(mux, uiPath, "./tmp")

	restURL := fmt.Sprintf("http://localhost:%d%s%s", addr.Port, uiPath, restRunURI)

	runURL := fmt.Sprintf("%s?qps=%d&url=%s&t=2s", restURL, 100, baseURL)

	code, bytes := fhttp.FetchURL(runURL)
	if code != http.StatusOK {
		t.Errorf("Got unexpected error code: URL %s: %v", runURL, code)
	}
	res := fhttp.HTTPRunnerResults{}
	err := json.Unmarshal(bytes, &res)
	if err != nil {
		t.Fatalf("Unable to deserialize results: %q: %v", string(bytes), err)
	}
	if res.RetCodes[200] != 0 {
		t.Errorf("Got unexpected 200s %d on base: %+v", res.RetCodes[200], res)
	}
	if res.RetCodes[404] != 2*100 { // 2s at 100qps == 200
		t.Errorf("Got unexpected 404s count %d on base: %+v", res.RetCodes[404], res)
	}
	runURL = fmt.Sprintf("%s?qps=%d&url=%s&n=200", restURL, 100, baseURL+"foo/bar?delay=20ms&status=200:100")
	// TODO: make a util function before the 3rd copy/pasta
	code, bytes = fhttp.FetchURL(runURL)
	if code != http.StatusOK {
		t.Errorf("Got unexpected error code: URL %s: %v", runURL, code)
	}
	res = fhttp.HTTPRunnerResults{}
	err = json.Unmarshal(bytes, &res)
	if err != nil {
		t.Fatalf("Unable to deserialize results: %q: %v", string(bytes), err)
	}
	totalReq := res.DurationHistogram.Count
	httpOk := res.RetCodes[http.StatusOK]
	if totalReq != httpOk {
		t.Errorf("Mismatch between requests %d and ok %v (%+v)", totalReq, res.RetCodes, res)
	}
	if res.SocketCount != res.RunnerResults.NumThreads {
		t.Errorf("%d socket used, expected same as thread# %d", res.SocketCount, res.RunnerResults.NumThreads)
	}
}
