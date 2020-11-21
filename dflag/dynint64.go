// Copyright 2015 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package dflag

import (
	"flag"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
)

// DynInt64 creates a `Flag` that represents `int64` which is safe to change dynamically at runtime.
func DynInt64(flagSet *flag.FlagSet, name string, value int64, usage string) *DynInt64Value {
	dynValue := &DynInt64Value{ptr: &value}
	flagSet.Var(dynValue, name, usage)
	return dynValue
}

// DynInt64Value is a flag-related `int64` value wrapper.
type DynInt64Value struct {
	DynamicFlagValueTag
	ptr       *int64
	validator func(int64) error
	notifier  func(oldValue int64, newValue int64)
}

// Get retrieves the value in a thread-safe manner.
func (d *DynInt64Value) Get() int64 {
	if d.ptr == nil {
		return 0
	}
	return atomic.LoadInt64(d.ptr)
}

// Set updates the value from a string representation in a thread-safe manner.
// This operation may return an error if the provided `input` doesn't parse, or the resulting value doesn't pass an
// optional validator.
// If a notifier is set on the value, it will be invoked in a separate go-routine.
func (d *DynInt64Value) Set(input string) error {
	val, err := strconv.ParseInt(strings.TrimSpace(input), 0, 64)
	if err != nil {
		return err
	}
	if d.validator != nil {
		if err := d.validator(val); err != nil {
			return err
		}
	}
	oldVal := atomic.SwapInt64(d.ptr, val)
	if d.notifier != nil {
		go d.notifier(oldVal, val)
	}
	return nil
}

// WithValidator adds a function that checks values before they're set.
// Any error returned by the validator will lead to the value being rejected.
// Validators are executed on the same go-routine as the call to `Set`.
func (d *DynInt64Value) WithValidator(validator func(int64) error) *DynInt64Value {
	d.validator = validator
	return d
}

// WithNotifier adds a function is called every time a new value is successfully set.
// Each notifier is executed in a new go-routine.
func (d *DynInt64Value) WithNotifier(notifier func(oldValue int64, newValue int64)) *DynInt64Value {
	d.notifier = notifier
	return d
}

// String returns the canonical string representation of the type.
func (d *DynInt64Value) String() string {
	return fmt.Sprintf("%v", d.Get())
}

// ValidateDynInt64Range returns a validator function that checks if the integer value is in range.
func ValidateDynInt64Range(fromInclusive int64, toInclusive int64) func(int64) error {
	return func(value int64) error {
		if value > toInclusive || value < fromInclusive {
			return fmt.Errorf("value %v not in [%v, %v] range", value, fromInclusive, toInclusive)
		}
		return nil
	}
}
