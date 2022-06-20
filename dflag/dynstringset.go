// Copyright 2015 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package dflag

import (
	"flag"
	"fmt"
)

// DynStringSet creates a `Flag` that represents `map[string]struct{}` which is safe to change dynamically at runtime.
// Unlike `pflag.StringSlice`, consecutive sets don't append to the slice, but override it.
func DynStringSet(flagSet *flag.FlagSet, name string, value []string, usage string) *DynStringSetValue {
	d := Dyn(flagSet, name, SetFromSlice(value), usage)
	return &DynStringSetValue{d}
}

// In order to have methods unique to this subtype... we extend/have the generic instantiated type:

// DynStringSetValue implements a dynamic set of strings.
type DynStringSetValue struct {
	*DynValue[Set[string]]
}

// Contains returns whether the specified string is in the flag.
func (d *DynStringSetValue) Contains(val string) bool {
	v := d.Get()
	_, ok := v[val]
	return ok
}

// String represents the canonical representation of the type.
func (d *DynStringSetValue) String() string {
	v := d.Get()
	arr := make([]string, 0, len(v))
	for k := range v {
		arr = append(arr, k)
	}
	return fmt.Sprintf("%v", arr)
}

func ValidateDynStringSetMinElements(count int) func(Set[string]) error {
	return ValidateDynSetMinElements[string](count)
}
