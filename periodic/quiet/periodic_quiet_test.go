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

package quiet

import (
	"sync"
	"testing"
	"time"

	"istio.io/fortio/log"
	"istio.io/fortio/periodic"
)

// TODO: figure out how to set the loglevel without triggering race test failure
// and thus not needing this seperate file

type TestCount struct {
	count *int64
	lock  *sync.Mutex
}

func (c *TestCount) Run(i int) {
	c.lock.Lock()
	(*c.count)++
	c.lock.Unlock()
	time.Sleep(50 * time.Millisecond)
}

func TestQuietMode(t *testing.T) {
	log.SetLogLevel(log.Error)
	var count int64
	var lock sync.Mutex
	c := TestCount{&count, &lock}
	o := periodic.RunnerOptions{
		QPS:        100000,
		Exactly:    11,
		NumThreads: 1,
	}
	r := periodic.NewPeriodicRunner(&o)
	r.Options().MakeRunners(&c)
	count = 0
	r.Run()
	if count != 11 {
		t.Errorf("Test executed unexpected number of times %d instead %d", count, 11)
	}
}
