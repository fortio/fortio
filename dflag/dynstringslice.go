// Copyright 2015 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package dflag

import (
	"flag"
)

type DynStringSliceValue = DynValue[[]string] // For backward compatibility

// DynStringSlice creates a `Flag` that represents `[]string` which is safe to change dynamically at runtime.
// Unlike `pflag.StringSlice`, consecutive sets don't append to the slice, but override it.
func DynStringSlice(flagSet *flag.FlagSet, name string, value []string, usage string) *DynStringSliceValue {
	return Dyn(flagSet, name, value, usage)
}

// ValidateDynStringSliceMinElements validates that the given string slice has at least x elements.
func ValidateDynStringSliceMinElements(count int) func([]string) error {
	return ValidateDynSliceMinElements[string](count)
}
