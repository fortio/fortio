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

// Package version for fortio holds version information and build information.
package version // import "fortio.org/fortio/version"
import (
	"fmt"
	"runtime"
	"runtime/debug"

	"fortio.org/fortio/log"
)

var (
	// The following are (re)computed in init().
	version     = "dev"
	longVersion = "unknown long"
	fullVersion = "unknown full"
)

// Short returns the 3 digit short version string Major.Minor.Patch[-pre]
// version.Short() is the overall project version (used to version json
// output too). "-pre" is added when the version doesn't match exactly
// a git tag or the build isn't from a clean source tree. (only standard
// dockerfile based build of a clean, tagged source tree should print "X.Y.Z"
// as short version).
func Short() string {
	return version
}

// Long returns the long version and build information.
// Format is "X.Y.X[-pre] YYYY-MM-DD HH:MM SHA[-dirty]" date and time is
// the build date (UTC), sha is the git sha of the source tree.
func Long() string {
	return longVersion
}

// Full returns the Long version + all the run time BuildInfo, ie
// all the dependent modules and version and hash as well.
func Full() string {
	return fullVersion
}

// Carefully manually tested all the combinations in pair with Dockerfile.

func init() { // nolint:gochecknoinits //we do need an init for this
	binfo, ok := debug.ReadBuildInfo()
	if !ok {
		log.Errf("fortio: unexpected but no build info available")
		return
	}
	v := binfo.Main.Version
	// '(devel)' messes up the release-tests paths
	if v != "(devel)" {
		version = v[1:] // skip leading v
	}
	longVersion = version + " " + binfo.Main.Sum + " " + binfo.GoVersion + " " + runtime.GOARCH + " " + runtime.GOOS
	fullVersion = fmt.Sprintf("%s\n%v", longVersion, binfo.String())
}
