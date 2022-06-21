// Copyright 2020 Laurent Demailly. All Rights Reserved.
// See LICENSE for licensing terms.

// Tests the fix for #375
// panic: runtime error: invalid memory address or nil pointer dereference
// through flag.isZeroValue() -> dyn .Get()

package dflag

import (
	"bytes"
	"flag"
	"fmt"
	"strings"
	"testing"
	"time"
)

type foo struct{}

// Started as the -h not crashing test but now also checks value name etc.
func TestDynFlagPrintDefaultsNotCrashing(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.PanicOnError)
	DynBool(fs, "a_boolean", false, "b")
	DynDuration(fs, "d", 0*time.Second, "d")
	DynFloat64(fs, "f", 0, "f")
	DynInt64(fs, "int_with_named_type", 0, "`some` value")
	DynJSON(fs, "j", &foo{}, "j")
	DynString(fs, "s", "", "s")
	DynStringSet(fs, "sset", []string{}, "sset")
	DynStringSlice(fs, "sslice", []string{}, "sslice")
	var b bytes.Buffer
	fs.SetOutput(&b)
	fs.PrintDefaults() // no crash/panic == test passes
	s := b.String()
	fmt.Printf("got output %q\n", s)
	good := 0
	for _, line := range strings.Split(s, "\n") {
		fmt.Printf("line is %q\n", line)
		if strings.Contains(line, "-a_boolean") {
			good++
			if strings.Contains(line, "value") {
				t.Errorf("line %q should not have value (boolean flag)", line)
			}
		} else if strings.Contains(line, "-int_with_named_type") {
			good++
			if !strings.Contains(line, "some") {
				t.Errorf("line %q should use the `` named param", line)
			}
		} else if strings.Contains(line, "-f ") {
			good++
			if !strings.Contains(line, "value") {
				t.Errorf("line %q should have \"value\" to show flag value is needed", line)
			}
		}
	}
	if good != 3 {
		t.Errorf("missing expected 3 flags tested in output, got %v", good)
	}
}
