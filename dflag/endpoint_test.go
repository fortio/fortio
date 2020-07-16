// Copyright 2015 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package flagz

import (
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type endpointTestSuite struct {
	suite.Suite
	flagSet  *flag.FlagSet
	endpoint *StatusEndpoint
}

func TestEndpointTestSuite(t *testing.T) {
	suite.Run(t, &endpointTestSuite{})
}

func (s *endpointTestSuite) SetupTest() {
	s.flagSet = flag.NewFlagSet("foobar", flag.ContinueOnError)
	s.endpoint = NewStatusEndpoint(s.flagSet)

	s.flagSet.String("some_static_string", "trolololo", "Some static string text")
	s.flagSet.Float64("some_static_float", 3.14, "Some static int text")

	DynStringSlice(s.flagSet, "some_dyn_stringslice", []string{"foo", "bar"}, "Some dynamic slice text")
	DynJSON(s.flagSet, "some_dyn_json", &testJSON{SomeString: "foo", SomeInt: 1337}, "Some dynamic JSON text")

	// Mark one static and one dynamic flag as changed.
	s.flagSet.Set("some_static_string", "yolololo")
	s.flagSet.Set("some_dyn_stringslice", "car,star")
}

func (s *endpointTestSuite) TestReturnsAll() {
	req, _ := http.NewRequest("GET", "/debug/flagz", nil)
	list := s.processFlagSetJSONResponse(req)
	s.assertListContainsOnly([]string{"some_static_string", "some_static_float", "some_dyn_stringslice", "some_dyn_json"}, list)
}

func (s *endpointTestSuite) TestReturnsOnlyChanged() {
	req, _ := http.NewRequest("GET", "/debug/flagz?only_changed=true", nil)
	list := s.processFlagSetJSONResponse(req)
	s.assertListContainsOnly([]string{"some_static_string", "some_dyn_stringslice"}, list)
}

func (s *endpointTestSuite) TestReturnsOnlyStatic() {
	req, _ := http.NewRequest("GET", "/debug/flagz?type=static", nil)
	list := s.processFlagSetJSONResponse(req)
	s.assertListContainsOnly([]string{"some_static_string", "some_static_float"}, list)
}

func (s *endpointTestSuite) TestReturnsOnlyDynamic() {
	req, _ := http.NewRequest("GET", "/debug/flagz?type=dynamic", nil)
	list := s.processFlagSetJSONResponse(req)
	s.assertListContainsOnly([]string{"some_dyn_stringslice", "some_dyn_json"}, list)
}

func (s *endpointTestSuite) TestReturnsOnlyDynamicAndChanged() {
	req, _ := http.NewRequest("GET", "/debug/flagz?type=dynamic&only_changed=true", nil)
	list := s.processFlagSetJSONResponse(req)
	s.assertListContainsOnly([]string{"some_dyn_stringslice"}, list)
}

func (s *endpointTestSuite) TestReturnsOnlyStaticAndChanged() {
	req, _ := http.NewRequest("GET", "/debug/flagz?type=static&only_changed=true", nil)
	list := s.processFlagSetJSONResponse(req)
	s.assertListContainsOnly([]string{"some_static_string"}, list)
}

func (s *endpointTestSuite) TestCorrectlyRepresentsResources() {
	req, _ := http.NewRequest("GET", "/debug/flagz", nil)
	list := s.processFlagSetJSONResponse(req)

	assert.Equal(s.T(),
		&flagJSON{
			Name:         "some_static_float",
			Description:  "Some static int text",
			CurrentValue: "3.14",
			DefaultValue: "3.14",
			IsChanged:    false,
			IsDynamic:    false,
		},
		findFlagInFlagSetJSON("some_static_float", list),
		"must correctly represent a static unchanged flag",
	)
	assert.Equal(s.T(),
		&flagJSON{
			Name:         "some_dyn_stringslice",
			Description:  "Some dynamic slice text",
			CurrentValue: "[car star]",
			DefaultValue: "[foo bar]",
			IsChanged:    true,
			IsDynamic:    true,
		},
		findFlagInFlagSetJSON("some_dyn_stringslice", list),
		"must correctly represent a dynamic changed flag",
	)
}

func (s *endpointTestSuite) TestServesHTML() {
	req, _ := http.NewRequest("GET", "/debug/flagz", nil)
	req.Header.Add("Accept", "application/xhtml+xml")
	resp := httptest.NewRecorder()
	s.endpoint.ListFlags(resp, req)
	require.Equal(s.T(), http.StatusOK, resp.Code, "flagz list request must return 200 OK")
	require.Contains(s.T(), resp.Header().Get("Content-Type"), "html", "must indicate html in content type")

	out := resp.Body.String()
	assert.Contains(s.T(), out, "<html>")
	assert.Contains(s.T(), out, "some_dyn_stringslice")
}

func (s *endpointTestSuite) processFlagSetJSONResponse(req *http.Request) *flagSetJSON {
	resp := httptest.NewRecorder()
	s.endpoint.ListFlags(resp, req)
	require.Equal(s.T(), http.StatusOK, resp.Code, "flagz list request must return 200 OK")
	require.Equal(s.T(), "application/json", resp.Header().Get("Content-Type"), "type must be indicated")
	ret := &flagSetJSON{}
	require.NoError(s.T(), json.Unmarshal(resp.Body.Bytes(), ret), "unmarshaling JSON response must succeed")
	return ret
}

func (s *endpointTestSuite) assertListContainsOnly(flagList []string, list *flagSetJSON) {
	existing := []string{}
	for _, f := range list.Flags {
		existing = append(existing, f.Name)
	}
	sort.Strings(flagList)
	require.EqualValues(s.T(), flagList, existing, "expected set of listed flags must match")
}

func findFlagInFlagSetJSON(flagName string, list *flagSetJSON) *flagJSON {
	for _, f := range list.Flags {
		if f.Name == flagName {
			return f
		}
	}
	return nil
}

type testJSON struct {
	SomeString string `json:"string"`
	SomeInt    int32  `json:"json"`
}
