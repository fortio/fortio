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

// This package has moved to fortio.org/log
package log // import "fortio.org/fortio/log"

import (
	"flag"
	"fmt"
	"strings"

	"fortio.org/dflag"
	"fortio.org/log"
)

// ChangeFlagsDefault sets some flags to a different default.
func ChangeFlagsDefault(newDefault string, flagNames ...string) {
	for _, flagName := range flagNames {
		f := flag.Lookup(flagName)
		if f == nil {
			log.Fatalf("flag %s not found", flagName)
			continue // not reached but linter doesn't know Fatalf panics/exits
		}
		f.DefValue = newDefault
		err := f.Value.Set(newDefault)
		if err != nil {
			log.Fatalf("error setting flag %s: %v", flagName, err)
		}
	}
}

//nolint:gochecknoinits // needed
func init() {
	// virtual dynLevel flag that maps back to actual level
	_ = dflag.DynString(flag.CommandLine, "loglevel", log.GetLogLevel().String(),
		fmt.Sprintf("loglevel, one of %v", log.LevelToStrA)).WithInputMutator(
		func(inp string) string {
			// The validation map has full lowercase and capitalized first letter version
			return strings.ToLower(strings.TrimSpace(inp))
		}).WithValidator(
		func(newStr string) error {
			_, err := log.ValidateLevel(newStr)
			return err
		}).WithSyncNotifier(
		func(old, newStr string) {
			_ = log.SetLogLevelStr(newStr) // will succeed as we just validated it first
		})
}
