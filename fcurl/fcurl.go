// Copyright 2018 Istio Authors
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

package main

// Do not add any external dependencies we want to keep fortio minimal.

import (
	"fortio.org/cli"
	"fortio.org/fortio/bincommon"
)

func main() {
	cli.ProgramName = "Φορτίο fortio-curl"
	cli.ArgsHelp = "url"
	cli.MinArgs = 1
	bincommon.SharedMain()
	cli.Main()
	o := bincommon.SharedHTTPOptions()
	bincommon.FetchURL(o)
}
