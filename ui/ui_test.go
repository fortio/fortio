// Copyright 2017 Istio Authors
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

package ui // import "istio.io/fortio/ui"

import (
	"testing"
)

func TestHTTPtoHTTPS(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		output string
	}{
		{
			"http prefixed url",
			"http://fortio.istio.io",
			"https://fortio.istio.io",
		},
		{
			"https prefixed url",
			"https://fortio.istio.io",
			"https://fortio.istio.io",
		},
		{
			"no prefix in url",
			"fortio.istio.io",
			"fortio.istio.io",
		},
	}

	for _, tc := range tests {
		pURL := httpToHTTPS(tc.input)
		if pURL != tc.output {
			t.Errorf("Test case %s failed to parse URL: %s\n\texpected: %s\n\t  actual: %s",
				tc.name,
				tc.input,
				tc.output,
				pURL,
			)
		}
	}
}
