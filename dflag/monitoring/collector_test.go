// Copyright 2015 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package monitoring_test

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ldemailly/go-flagz"
	"github.com/ldemailly/go-flagz/monitoring"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type monitoringTestSuite struct {
	suite.Suite
	flagSet     *flag.FlagSet
	setName     string
	promHandler http.Handler
}

func TestMonitoringTestSuite(t *testing.T) {
	suite.Run(t, &monitoringTestSuite{promHandler: promhttp.Handler()})
}

func (s *monitoringTestSuite) SetupTest() {
	s.setName = fmt.Sprintf("set%d", rand.Int31()%128)
	s.flagSet = flag.NewFlagSet(s.setName, flag.ContinueOnError)
	monitoring.MustRegisterFlagSet(s.setName, s.flagSet)

	s.flagSet.String("some_static_string", "trolololo", "Some static string text")
	s.flagSet.Float64("some_static_float", 3.14, "Some static int text")
	flagz.DynInt64(s.flagSet, "some_dyn_int", 1337, "Something dynamic")
	flagz.DynString(s.flagSet, "some_dyn_string", "yolololo", "Something dynamic")
}

func (s *monitoringTestSuite) TestJustReads() {
	out := s.fetchPrometheusLines(s.setName)
	require.Equal(s.T(), 2, len(out), "two separate time series for a single set")
}

func (s *monitoringTestSuite) TestChangesOnStatic() {
	before := s.fetchPrometheusLines(s.setName)
	s.flagSet.Set("some_static_string", "i_am_changed")
	after := s.fetchPrometheusLines(s.setName)

	equalLines := findEqualStrings(before, after)
	require.Equal(s.T(), 1, len(equalLines), "only the dynamic checksum should not change")
	require.Contains(s.T(), equalLines[0], "dynamic")
}

func (s *monitoringTestSuite) TestChangesOnDynamic() {
	before := s.fetchPrometheusLines(s.setName)
	s.flagSet.Set("some_dyn_int", "707070")
	after := s.fetchPrometheusLines(s.setName)

	equalLines := findEqualStrings(before, after)
	require.Equal(s.T(), 1, len(equalLines), "only the static checksum should not change")
	require.Contains(s.T(), equalLines[0], "static")
}

func (s *monitoringTestSuite) fetchPrometheusLines(setName string) []string {
	resp := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/", nil)
	require.NoError(s.T(), err, "failed creating request")
	s.promHandler.ServeHTTP(resp, req)
	reader := bufio.NewReader(resp.Body)
	ret := []string{}
	for {
		line, err := reader.ReadString('\n')
		if err == io.EOF {
			break
		} else {
			require.NoError(s.T(), err, "error reading stuff")
		}
		if strings.Contains(line, `"`+setName+`"`) {
			ret = append(ret, line)
		}
	}
	return ret
}

func findEqualStrings(set1 []string, set2 []string) []string {
	ret := []string{}
	for _, s1 := range set1 {
		for _, s2 := range set2 {
			if s1 == s2 {
				ret = append(ret, s1)
			}
		}
	}
	return ret
}
