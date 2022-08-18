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
package version // import "fortio.org/fortio/version"
import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"

	"fortio.org/fortio/log"
)

var (
	// The following are (re)computed in init().
	version     = "dev"
	longVersion = "unknown long"
	fullVersion = "unknown full"
)

// Short returns the 3 digit short fortio version string Major.Minor.Patch
// it matches the project git tag (without the leading v) or "dev" when
// not built from tag / not `go install fortio.org/fortio@latest`
// version.Short() is the overall project version (used to version json
// output too).
func Short() string {
	return version
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

// FromBuildInfo can be called by other programs to get their version strings (short,long and full)
// automatically added by go 1.18+ when doing `go install project@vX.Y.Z`
// and is also used for fortio itself.
func FromBuildInfo() (short, long, full string) {
	return FromBuildInfoPath("")
}

func getVersion(binfo *debug.BuildInfo, path string) (short, sum string) {
	if path == "" || binfo.Main.Path == path {
		// skip leading v, assumes the project use `vX.Y.Z` tags.
		short = strings.TrimLeft(binfo.Main.Version, "v")
		// '(devel)' messes up the release-tests paths
		if short == "(devel)" || short == "" {
			short = "dev"
		}
		sum = binfo.Main.Sum
		return
	}
	// try to find the right module in deps
	short = path + " not found in buildinfo"
	for _, m := range binfo.Deps {
		if path == m.Path {
			short = strings.TrimLeft(m.Version, "v")
			sum = m.Sum
			return
		}
	}
	return
}

// FromBuildInfoPath returns the version of as specific module if that module isn't already the main one.
// Used by Fortio library version init to remember it's own version.
func FromBuildInfoPath(path string) (short, long, full string) {
	binfo, ok := debug.ReadBuildInfo()
	if !ok {
		full = "fortio version module error, no build info"
		log.Errf(full)
		return
	}
	short, sum := getVersion(binfo, path)
	long = short + " " + sum + " " + binfo.GoVersion + " " + runtime.GOARCH + " " + runtime.GOOS
	full = fmt.Sprintf("%s\n%v", long, binfo.String())
	return
}

// This "burns in" the fortio version. we need to get the "right" versions though.
// depending if we are a module or main.
func init() { //nolint:gochecknoinits // we do need an init for this
	version, longVersion, fullVersion = FromBuildInfoPath("fortio.org/fortio")
}
