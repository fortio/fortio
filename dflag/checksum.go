// Copyright 2015 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package dflag

import (
	"flag"
	"hash/fnv"
	"sync"
)

// @todo Temporary fix for race condition happening in the test:
// spf13/pflag.FlagSet.VisitAll seems to be prone to a race condition
// This fixes that but I'm not sure how much slower does it make the codebase.
var visitAllMutex = &sync.Mutex{}

// ChecksumFlagSet will generate a FNV of the *set* values in a FlagSet.
func ChecksumFlagSet(flagSet *flag.FlagSet, flagFilter func(flag *flag.Flag) bool) []byte {
	h := fnv.New32a()

	visitAllMutex.Lock()
	defer visitAllMutex.Unlock()
	flagSet.VisitAll(func(flag *flag.Flag) {
		if flagFilter != nil && !flagFilter(flag) {
			return
		}
		_, _ = h.Write([]byte(flag.Name))
		_, _ = h.Write([]byte(flag.Value.String()))
	})
	return h.Sum(nil)
}
