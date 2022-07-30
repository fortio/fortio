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

package jrpc // import "fortio.org/fortio/jrpc"

// Server side additional code (compared to restClient.go).
import (
	"encoding/json"
	"io/ioutil"
	"net/http"
)

type ReplyMessage struct {
	Failed  bool // would be named Success if could default it to true
	Message string
}

type ErrorReply struct {
	ReplyMessage
	Exception string
}

func NewErrorReply(message string, err error) *ErrorReply {
	res := ErrorReply{}
	res.Failed = true
	res.Message = message
	if err != nil {
		res.Exception = err.Error()
	}
	return &res
}

// Reply a struct as json (or just writes desired code).
func Reply[T any](w http.ResponseWriter, code int, data *T) error {
	var bytes []byte
	var err error
	if data == nil {
		w.WriteHeader(code)
		return nil
	}
	w.Header().Set("Content-Type", "application/json")
	bytes, err = json.Marshal(data)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return err
	}
	w.WriteHeader(code)
	_, err = w.Write(bytes)
	return err
}

func ReplyNoPayload(w http.ResponseWriter, code int) error {
	return Reply[int](w, code, nil)
}

func ReplyOk[T any](w http.ResponseWriter, data *T) error {
	return Reply(w, http.StatusOK, data)
}

func ReplyClientError[T any](w http.ResponseWriter, data *T) error {
	return Reply(w, http.StatusBadRequest, data)
}

func ReplyServerError[T any](w http.ResponseWriter, data *T) error {
	return Reply(w, http.StatusServiceUnavailable, data)
}

func ReplyError(w http.ResponseWriter, extraMsg string, err error) error {
	return ReplyClientError(w, NewErrorReply(extraMsg, err))
}

func Deserialize[Q any](bytes []byte) (*Q, error) {
	var result Q
	err := json.Unmarshal(bytes, &result)
	return &result, err // Will return zero object, not nil upon error
}

func HandleCall[Q any](w http.ResponseWriter, r *http.Request) (*Q, error) {
	data, err := ioutil.ReadAll(r.Body) // must be done before calling FormValue
	if err != nil {
		return nil, err
	}
	return Deserialize[Q](data)
}
