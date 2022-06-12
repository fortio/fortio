// Copyright 2015 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package dflag

import (
	"flag"
)

// DynInt64 creates a `Flag` that represents `int64` which is safe to change dynamically at runtime.
func DynInt64(flagSet *flag.FlagSet, name string, value int64, usage string) *DynValue[int64] {
	return Dyn(flagSet, name, value, usage)
}

// ValidateDynInt64Range returns a validator function that checks if the integer value is in range.
func ValidateDynInt64Range(fromInclusive int64, toInclusive int64) func(int64) error {
	return ValidateRange(fromInclusive, toInclusive)
}
