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

package ui

import (
	"bytes"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestSetHostAndPort(t *testing.T) {
	var tests = []struct {
		inputPort string
		addr      *net.TCPAddr
		expected  string
	}{
		{":8080", &net.TCPAddr{
			[]byte{192, 168, 2, 3},
			8081,
			"",
		}, "192.168.2.3:8081"},
		{":8081", &net.TCPAddr{
			[]byte{192, 168, 30, 14},
			8080,
			"",
		}, "192.168.30.14:8080"},
		{":8080",
			nil,
			"localhost:8080"},
		{"",
			&net.TCPAddr{
				[]byte{192, 168, 30, 14},
				9090,
				"",
			}, "192.168.30.14:9090"},
	}
	for _, test := range tests {
		setHostAndPort(test.inputPort, test.addr)
		if urlHostPort != test.expected {
			t.Errorf("%s was expected but %s is received ", test.expected, urlHostPort)
		}
	}
}

func TestResultToJsData(t *testing.T) {
	var tests = []struct {
		jsonbyte []byte
		expected string
	}{
		{
			[]byte("{\"test\":\"testing\"}"),
			"var res = {\"test\":\"testing\"}\nvar data = fortioResultToJsChartData(res)\nshowChart(data)\n",
		},
	}

	for _, test := range tests {
		var tB bytes.Buffer
		ResultToJsData(&tB, test.jsonbyte)
		if test.expected != tB.String() {
			t.Errorf("%s was expected but %s is received ", test.expected, tB.String())
		}
	}
}

func TestSaveJSON(t *testing.T) {
	dataDirT := "test"
	os.MkdirAll(dataDirT, os.ModePerm)
	var tests = []struct {
		name     string
		jsonbyte []byte
		dataDir  string
		expected string
	}{
		{
			"test",
			[]byte("{\"test\":\"testing\"}"),
			dataDirT,
			"data/test.json",
		},
		{
			"test",
			[]byte("{\"test\":\"testing\"}"),
			"",
			"",
		},
	}
	for _, test := range tests {
		dataDir = test.dataDir
		fileName := SaveJSON(test.name, test.jsonbyte)
		if test.expected != fileName {
			t.Errorf("%s was expected but %s is received ", test.expected, fileName)
		}
	}
	os.RemoveAll(dataDirT)
}

func TestDataList(t *testing.T) {
	dataDirT := "test"
	os.MkdirAll(dataDirT, os.ModePerm)
	var tests = []struct {
		name     string
		opType   string
		expected string
	}{
		{
			"test.json",
			"file",
			"test",
		},
		{
			"test",
			"dir",
			"",
		},
		{
			"test1.json",
			"file",
			"test1",
		},
	}
	expectedResults := make([]string, 0)
	for _, test := range tests {
		if test.opType == "dir" {
			os.MkdirAll(path.Join(dataDirT, test.name), os.ModePerm)
		} else {
			expectedResults = append(expectedResults, test.expected)
			ioutil.WriteFile(path.Join(dataDirT, test.name), []byte{}, 0644)
		}
	}
	dataDir = dataDirT
	results := DataList()
	if len(results) != len(expectedResults) {
		t.Errorf("Expected result size does not matched with results. Expected was %v. Result was %v", expectedResults, results)
	}
	sort.Strings(results)
	sort.Strings(expectedResults)
	if !reflect.DeepEqual(results, expectedResults) {
		t.Errorf("%v is expected to be equal to %v", results, expectedResults)
	}
	os.RemoveAll(dataDirT)
}

func TestServe(t *testing.T) {
	var tests = []struct {
		baseurl                string
		port                   string
		debugpath              string
		uipath                 string
		staticRsrcDir          string
		datadir                string
		percentileList         []float64
		expectedPercentileList []float64
		expectedDataDir        string
		expectedUiPath         string
	}{
		{
			"", "9090", "", "/fortio/", "", "test",
			[]float64{50, 75, 90, 99, 99.9}, []float64{50, 75, 90, 99, 99.9},
			"test", "/fortio/",
		},
		{
			"", "9091", "", "/fortio", "", "test",
			[]float64{}, []float64{},
			"test", "/fortio/",
		},
		{
			"", "9092", "", "", "", "test",
			[]float64{75, 90, 99}, []float64{},
			"", "",
		},
	}
	for _, test := range tests {
		defaultPercentileList = []float64{}
		dataDir = ""
		uiPath = ""
		Serve(test.baseurl, test.port, test.debugpath, test.uipath, test.staticRsrcDir, test.datadir, test.percentileList)
		if !reflect.DeepEqual(defaultPercentileList, test.expectedPercentileList) {
			t.Errorf("%v is expected to be equal to %v", defaultPercentileList, test.percentileList)
		}
		if dataDir != test.expectedDataDir {
			t.Errorf("%v is expected to be equal to %v", dataDir, test.datadir)
		}
		if uiPath != test.expectedUiPath {
			t.Errorf("%v is expected to be equal to %v", uiPath, test.uipath)
		}
	}
}

func TestReport(t *testing.T) {
	var tests = []struct {
		baseurl             string
		port                string
		staticRsrcDir       string
		datadir             string
		expectedUiPath      string
		expectedUrlHostPort string
	}{
		{
			"", "8080", "test", "test", "/", "[::]:8080",
		},
	}
	for _, test := range tests {
		dataDir = ""
		uiPath = ""
		Report(test.baseurl, test.port, test.staticRsrcDir, test.datadir)
		if dataDir != test.datadir {
			t.Errorf("%v is expected to be equal to %v", dataDir, test.datadir)
		}
		if uiPath != test.expectedUiPath {
			t.Errorf("%v is expected to be equal to %v", uiPath, test.expectedUiPath)
		}
		if urlHostPort != test.expectedUrlHostPort {
			t.Errorf("%v is expected to be equal to %v", urlHostPort, test.expectedUrlHostPort)
		}
	}
}

//http://localhost:8080/fortio/?labels=Fortio&url=http%3A%2F%2Flocalhost%3A8080%2Fecho&t=3s&qps=1000&save=on&r=0.0001&load=Start
func TestPercentilesForHandler(t *testing.T) {
	percentileListT := []float64{50, 75, 90, 99, 99.9}
	uiPathT := "/fortio"
	baseURLT := ""
	portT := "8181"
	var tests = []struct {
		url                   string
		expectedJsonBodyTexts []string
		expectedHtmlBodyTexts []string
	}{
		{
			"http://localhost:8181/fortio/?labels=Fortio&url=http%3A%2F%2Flocalhost%3A8181%2Fecho&t=3s&qps=1000&save=on&r=0.0001&load=Start",
			[]string{"\"Percentile\": 50", "\"Percentile\": 75", "\"Percentile\": 90", "\"Percentile\": 99", "\"Percentile\": 99.9"},
			[]string{"target 50%", "target 75%", "target 90%", "target 99%", "target 99.9%"},
		},
		{
			"http://localhost:8181/fortio/?labels=Fortio&url=http%3A%2F%2Flocalhost%3A8181%2Fecho&t=3s&qps=1000&save=on&r=0.0001&p=&load=Start",
			[]string{},
			[]string{},
		},
		{
			"http://localhost:8181/fortio/?labels=Fortio&url=http%3A%2F%2Flocalhost%3A8181%2Fecho&t=3s&qps=1000&save=on&r=0.0001&p=50,60,99&load=Start",
			[]string{"\"Percentile\": 50", "\"Percentile\": 60", "\"Percentile\": 99"},
			[]string{"target 50%", "target 60", "target 99"},
		},
	}
	Serve(baseURLT, portT, "", uiPathT, "", "", percentileListT)
	for _, test := range tests {
		resp, err := http.Get(test.url)
		if err != nil {
			log.Fatalf("Error is occurred while %s. Error message: %v", test.url, err)
		}
		if resp != nil {
			checkResponseBodyForPercentiles(t, resp, test.expectedJsonBodyTexts, test.expectedHtmlBodyTexts)
		}
	}
}

func checkResponseBodyForPercentiles(t *testing.T, res *http.Response, expectedJsonBodyTexts []string, expectedHtmlBodyTexts []string) {
	b, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Fatalf("while decoding the response, error is occured: %v", err)
	}
	bodyText := string(b)
	for _, expectedText := range expectedJsonBodyTexts {
		if !strings.Contains(bodyText, expectedText) {
			t.Errorf("%s was expected to be in %s", expectedText, bodyText)
		}
	}
	for _, expectedText := range expectedHtmlBodyTexts {
		if !strings.Contains(bodyText, expectedText) {
			t.Errorf("%s was expected to be in %s", expectedText, bodyText)
		}
	}
}
