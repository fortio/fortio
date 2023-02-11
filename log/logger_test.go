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

package log

import (
	"flag"
	"testing"

	"fortio.org/log"
)

func TestSetLevelFLag(t *testing.T) {
	_ = log.SetLogLevel(log.Info)
	err := flag.CommandLine.Set("loglevel", "  deBUG\n")
	if err != nil {
		t.Errorf("unexpected error for valid level %v", err)
	}
	prev := log.SetLogLevel(log.Info)
	if prev != log.Debug {
		t.Errorf("unexpected level after setting debug %v", prev)
	}
	err = flag.CommandLine.Set("loglevel", "bogus")
	if err == nil {
		t.Errorf("Didn't get an error setting bogus level")
	}
}

func TestChangeFlagsDefaultErrCase1(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected a panic from log.Fatalf, didn't get one")
		}
	}()
	ChangeFlagsDefault("value", "nosuchflag")
}

func TestChangeFlagsDefaultErrCase2(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected a panic from log.Fatalf, didn't get one")
		}
	}()
	ChangeFlagsDefault("foo", "loglevel")
}
