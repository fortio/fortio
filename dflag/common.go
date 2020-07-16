// Copyright 2015 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package dflag

import (
	"flag"
)

type DynamicFlagValue interface {
	IsDynamicFlag() bool
}

type DynamicJsonFlagValue interface {
	IsJson() bool
}

type DynamicFlagValueTag struct{}

func (*DynamicFlagValueTag) IsDynamicFlag() bool {
	return true
}

// A flag is dynamic if it implements DynamicFlagValue (which all the dyn* do)

// IsFlagDynamic returns whether the given Flag has been created in a Dynamic mode.
func IsFlagDynamic(f *flag.Flag) bool {
	_, ok := f.Value.(DynamicFlagValue)
	return ok
}

// TODO: consider caching this
func IsFlagSet(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}
