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
	IsJSON() bool
}

type DynamicFlagValueTag struct{}

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
