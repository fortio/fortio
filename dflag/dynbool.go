// Copyright (c) Improbable Worlds Ltd, Fortio Authors. All Rights Reserved
// See LICENSE for licensing terms.

package dflag

import (
	"flag"
	"fmt"
)

// Would use just
// type DynBoolValue = DynValue[bool]
// but that doesn't work with IsBoolFlag
// https://github.com/golang/go/issues/53473
// so we extend this type as special, the only one with that method.

// DynBool creates a `Flag` that represents `bool` which is safe to change dynamically at runtime.
func DynBool(flagSet *flag.FlagSet, name string, value bool, usage string) *DynBoolValue {
	dynValue := DynBoolValue{}
	dynInit(&dynValue.DynValue, flagSet, name, value, usage)
	flagSet.Var(&dynValue, name, usage)
	flagSet.Lookup(name).DefValue = fmt.Sprintf("%v", value)
	return &dynValue
}

// DynStringSetValue implements a dynamic set of strings.
type DynBoolValue struct {
	DynamicBoolValueTag
	DynValue[bool]
}
