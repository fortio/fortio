// Copyright 2020 Laurent Demailly
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

package fnet_test // really is a logger test but need to avoid circular dependencies...

import (
	"flag"
	"io/ioutil"
	"os"
	"path"
	"testing"
	"time"

	"fortio.org/fortio/dflag/configmap"
	"fortio.org/fortio/log"
)

func init() {
	log.SetFlagDefaultsForClientTools()
}

func TestDynamicLogLevel(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "fortio-logger-test")
	if err != nil {
		t.Fatalf("unexpected error getting tempdir %v", err)
	}
	defer os.RemoveAll(tmpDir)
	pDir := path.Join(tmpDir, "config")
	if err = os.Mkdir(pDir, 0o755); err != nil {
		t.Fatalf("unable to make %v: %v", pDir, err)
	}
	var u *configmap.Updater
	log.SetLogLevel(log.Debug)
	if u, err = configmap.Setup(flag.CommandLine, pDir); err != nil {
		t.Fatalf("unexpected error setting up config watch: %v", err)
	}
	defer u.Stop()
	fName := path.Join(pDir, "loglevel")
	// Test also the new normalization (space trimmimg and captitalization)
	if err = ioutil.WriteFile(fName, []byte(" InFO\n\n"), 0o644); err != nil {
		t.Fatalf("unable to write %v: %v", fName, err)
	}
	time.Sleep(1 * time.Second)
	newLevel := log.GetLogLevel()
	if newLevel != log.Info {
		t.Errorf("Loglevel didn't change as expected, still %v %v", newLevel, newLevel.String())
	}
	// put back debug
	log.SetLogLevel(log.Debug)
}
