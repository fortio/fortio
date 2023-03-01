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

package fnet

import (
	"bytes"
	"testing"
)

func TestStdinPayload(t *testing.T) {
	expected := []byte("test stdin")
	stdin = bytes.NewReader(expected)

	data, err := ReadFileForPayload("-")
	if err != nil {
		t.Errorf("Error should not be happened for ReadFileForPayload stdin: %v", err)
	}
	if !bytes.Equal(data, expected) {
		t.Errorf("Got %s, expected %s for ReadFileForPayload()", string(data), string(expected))
	}
}
