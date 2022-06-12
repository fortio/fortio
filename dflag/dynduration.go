// Copyright 2015 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package dflag

import (
	"flag"
	"time"
)

// DynDuration creates a `Flag` that represents `time.Duration` which is safe to change dynamically at runtime.
func DynDuration(flagSet *flag.FlagSet, name string, value time.Duration, usage string) *DynValue[time.Duration] {
	return Dyn(flagSet, name, value, usage)
}
