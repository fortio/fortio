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
	"io"
	"net/http"
)

type ServerReply struct {
	Error     bool   `json:"error,omitempty"` // Success if false/omitted, Error/Failure when true
	Message   string `json:"message,omitempty"`
	Exception string `json:"exception,omitempty"`
}

func NewErrorReply(message string, err error) *ServerReply {
	res := ServerReply{Error: true, Message: message}
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
	bytes, err = Serialize(data)
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

func HandleCall[Q any](w http.ResponseWriter, r *http.Request) (*Q, error) {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	return Deserialize[Q](data)
}
