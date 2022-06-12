// Copyright (c) Improbable Worlds Ltd, Fortio Authors. All Rights Reserved
// See LICENSE for licensing terms.

package dflag

import (
	"flag"
)

// type DynBoolValue DynValue[bool]

// DynBool creates a `Flag` that represents `bool` which is safe to change dynamically at runtime.
func DynBool(flagSet *flag.FlagSet, name string, value bool, usage string) *DynValue[bool] {
	return Dyn(flagSet, name, value, usage)
}
