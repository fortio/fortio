// Copyright 2015 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package dflag

import (
	"flag"
)

// DynFloat64 creates a `Flag` that represents `float64` which is safe to change dynamically at runtime.
func DynFloat64(flagSet *flag.FlagSet, name string, value float64, usage string) *DynValue[float64] {
	return Dyn(flagSet, name, value, usage)
}

// ValidateDynFloat64Range returns a validator that checks if the float value is in range.
func ValidateDynFloat64Range(fromInclusive float64, toInclusive float64) func(float64) error {
	return ValidateRange(fromInclusive, toInclusive)
}
