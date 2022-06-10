package ui // import "fortio.org/fortio/ui"

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
)

const emptyResult = `{"results":null}`

func TestEmptyResult(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/fortio/rest/status", nil)
	w := httptest.NewRecorder()
	RESTStatusHandler(w, r)
	res := w.Result()
	defer res.Body.Close()
	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Errorf("expected error to be nil got %v", err)
	}
	if string(data) != emptyResult {
		t.Errorf("expected %s got %v", emptyResult, string(data))
	}
}
