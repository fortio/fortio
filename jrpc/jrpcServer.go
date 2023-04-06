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

// Server side additional code (compared to jrpcClient.go).

import (
	"io"
	"net/http"
)

// ServerReply is used to reply errors but can also be the base for Ok replies,
// see the unit tests `Response` and ../rapi for examples of use.
type ServerReply struct {
	Error     bool   `json:"error,omitempty"` // Success if false/omitted, Error/Failure when true
	Message   string `json:"message,omitempty"`
	Exception string `json:"exception,omitempty"`
}

// NewErrorReply creates a new error reply with the message and err error.
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

// ReplyNoPayload is a short cut for Reply() with empty (nil) payload.
func ReplyNoPayload(w http.ResponseWriter, code int) error {
	return Reply[int](w, code, nil)
}

// ReplyOk is a short cut for Reply() with http.StatusOK as the result code.
func ReplyOk[T any](w http.ResponseWriter, data *T) error {
	return Reply(w, http.StatusOK, data)
}

// ReplyClientError is a short cut for Reply() with http.StatusBadRequest as the result code.
func ReplyClientError[T any](w http.ResponseWriter, data *T) error {
	return Reply(w, http.StatusBadRequest, data)
}

// ReplyServerError is a short cut for Reply() with http.StatusServiceUnavailable as the result code.
func ReplyServerError[T any](w http.ResponseWriter, data *T) error {
	return Reply(w, http.StatusServiceUnavailable, data)
}

// ReplyError is to send back a client error with exception details.
func ReplyError(w http.ResponseWriter, extraMsg string, err error) error {
	return ReplyClientError(w, NewErrorReply(extraMsg, err))
}

// ProcessRequest deserializes the expected type from the request body.
// Sample usage code:
//
//	req, err := jrpc.ProcessRequest[Request](w, r)
//	if err != nil {
//	    _ = jrpc.ReplyError(w, "request error", err)
//	}
func ProcessRequest[Q any](r *http.Request) (*Q, error) {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	return Deserialize[Q](data)
}

// Deprecated: use ProcessRequest instead.
func HandleCall[Q any](_ http.ResponseWriter, r *http.Request) (*Q, error) {
	return ProcessRequest[Q](r)
}
