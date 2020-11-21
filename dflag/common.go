// Copyright 2015 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package dflag

import (
	"flag"
)

// DynamicFlagValue interface is a tag to know if a type is dynamic or not.
type DynamicFlagValue interface {
	IsDynamicFlag() bool
}

// DynamicJSONFlagValue is a tag interface for JSON dynamic flags.
type DynamicJSONFlagValue interface {
	IsJSON() bool
}

// DynamicFlagValueTag is a struct all dynamic flag inherit for marking they are dynamic.
type DynamicFlagValueTag struct{}

// IsDynamicFlag always returns true.
func (*DynamicFlagValueTag) IsDynamicFlag() bool {
	return true
}

// A flag is dynamic if it implements DynamicFlagValue (which all the dyn* do)

// IsFlagDynamic returns whether the given Flag has been created in a Dynamic mode.
func IsFlagDynamic(f *flag.Flag) bool {
	df, ok := f.Value.(DynamicFlagValue)
	if !ok {
		return false
	}
	return df.IsDynamicFlag() // will clearly return true if it exists
}
