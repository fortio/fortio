// Copyright 2017 Fortio Authors
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

// Package version for fortio holds version information and build information.
// The reusable library part and examples moved to [fortio.org/version].
package version // import "fortio.org/fortio/version"
import (
	"fortio.org/version"
)

var (
	// The following are (re)computed in init().
	shortVersion = "dev"
	longVersion  = "unknown long"
	fullVersion  = "unknown full"
)

// Short returns the 3 digit short fortio version string Major.Minor.Patch
// it matches the project git tag (without the leading v) or "dev" when
// not built from tag / not `go install fortio.org/fortio@latest`
// version.Short() is the overall project version (used to version json
// output too).
func Short() string {
	return shortVersion
}

// Long returns the long fortio version and build information.
// Format is "X.Y.X hash go-version processor os".
func Long() string {
	return longVersion
}

// Full returns the Long version + all the run time BuildInfo, ie
// all the dependent modules and version and hash as well.
func Full() string {
	return fullVersion
}

// This "burns in" the fortio version. we need to get the "right" versions though.
// depending if we are a module or main.
func init() { //nolint:gochecknoinits // we do need an init for this
	shortVersion, longVersion, fullVersion = version.FromBuildInfoPath("fortio.org/fortio")
}
