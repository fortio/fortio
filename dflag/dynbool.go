// Copyright (c) Improbable Worlds Ltd, All Rights Reserved
// See LICENSE for licensing terms.

package dflag

import (
	"flag"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
)

// DynBool creates a `Flag` that represents `bool` which is safe to change dynamically at runtime.
func DynBool(flagSet *flag.FlagSet, name string, value bool, usage string) *DynBoolValue {
	v := boolToInt(value)
	dynValue := &DynBoolValue{ptr: &v}
	flagSet.Var(dynValue, name, usage)
	flagSet.Lookup(name).DefValue = strconv.FormatBool(value)
	return dynValue
}

// DynBoolValue is a flag-related `int64` value wrapper.
type DynBoolValue struct {
	DynamicFlagValueTag
	ptr       *uint32
	validator func(bool) error
	notifier  func(oldValue bool, newValue bool)
}

// IsBoolFlag lets the flag parsing know that -flagname is enough to turn to true.
func (d *DynBoolValue) IsBoolFlag() bool {
	return true
}

// Get retrieves the value in a thread-safe manner.
func (d *DynBoolValue) Get() bool {
	if d.ptr == nil {
		return false
	}
	return atomic.LoadUint32(d.ptr) == 1
}

// Set updates the value from a string representation in a thread-safe manner.
// This operation may return an error if the provided `input` doesn't parse, or the resulting value doesn't pass an
// optional validator.
// If a notifier is set on the value, it will be invoked in a separate go-routine.
func (d *DynBoolValue) Set(input string) error {
	val, err := strconv.ParseBool(strings.TrimSpace(input))
	if err != nil {
		return err
	}
	if d.validator != nil {
		if err := d.validator(val); err != nil {
			return err
		}
	}

	oldVal := atomic.SwapUint32(d.ptr, boolToInt(val))
	if d.notifier != nil {
		go d.notifier(oldVal == 1, val)
	}
	return nil
}

// WithValidator adds a function that checks values before they're set.
// Any error returned by the validator will lead to the value being rejected.
// Validators are executed on the same go-routine as the call to `Set`.
func (d *DynBoolValue) WithValidator(validator func(bool) error) {
	d.validator = validator
}

// WithNotifier adds a function is called every time a new value is successfully set.
// Each notifier is executed in a new go-routine.
func (d *DynBoolValue) WithNotifier(notifier func(oldValue bool, newValue bool)) {
	d.notifier = notifier
}

// Type is an indicator of what this flag represents.
func (d *DynBoolValue) Type() string {
	return "dyn_bool"
}

// String returns the canonical string representation of the type.
func (d *DynBoolValue) String() string {
	return fmt.Sprintf("%v", d.Get())
}

func boolToInt(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}
