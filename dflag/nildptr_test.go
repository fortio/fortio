// Copyright 2020 Laurent Demailly. All Rights Reserved.
// See LICENSE for licensing terms.

// Tests the fix for #375
// panic: runtime error: invalid memory address or nil pointer dereference
// through flag.isZeroValue() -> dyn .Get()

package dflag

import (
	"testing"
	"time"

	"flag"
)

type foo struct {
}

func TestDynFlagPrintDefaultsNotCrashing(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.PanicOnError)
	DynBool(fs, "b", false, "b")
	DynDuration(fs, "d", 0*time.Second, "d")
	DynFloat64(fs, "f", 0, "f")
	DynInt64(fs, "i", 0, "i")
	DynJSON(fs, "j", &foo{}, "j")
	DynString(fs, "s", "", "s")
	DynStringSet(fs, "sset", []string{}, "sset")
	DynStringSlice(fs, "sslice", []string{}, "sslice")
	fs.PrintDefaults() // no crash/panic == test passes
	// TODO potentially, check the output and the zero valued versus not help string difference
}
