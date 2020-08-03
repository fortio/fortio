// Copyright 2015 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package dflag

import (
	"flag"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"unsafe"
)

// DynFloat64 creates a `Flag` that represents `float64` which is safe to change dynamically at runtime.
func DynFloat64(flagSet *flag.FlagSet, name string, value float64, usage string) *DynFloat64Value {
	dynValue := &DynFloat64Value{ptr: unsafe.Pointer(&value)}
	flagSet.Var(dynValue, name, usage)
	return dynValue
}

// DynFloat64Value is a flag-related `float64` value wrapper.
type DynFloat64Value struct {
	DynamicFlagValueTag
	ptr       unsafe.Pointer
	validator func(float64) error
	notifier  func(oldValue float64, newValue float64)
}

// Get retrieves the value in a thread-safe manner.
func (d *DynFloat64Value) Get() float64 {
	if d.ptr == nil {
		return 0.0
	}
	p := (*float64)(atomic.LoadPointer(&d.ptr))
	return *p
}

// Set updates the value from a string representation in a thread-safe manner.
// This operation may return an error if the provided `input` doesn't parse, or the resulting value doesn't pass an
// optional validator.
// If a notifier is set on the value, it will be invoked in a separate go-routine.
func (d *DynFloat64Value) Set(input string) error {
	val, err := strconv.ParseFloat(strings.TrimSpace(input), 64)
	if err != nil {
		return err
	}
	if d.validator != nil {
		if err := d.validator(val); err != nil {
			return err
		}
	}
	oldPtr := atomic.SwapPointer(&d.ptr, unsafe.Pointer(&val))
	if d.notifier != nil {
		go d.notifier(*(*float64)(oldPtr), val)
	}
	return nil
}

// WithValidator adds a function that checks values before they're set.
// Any error returned by the validator will lead to the value being rejected.
// Validators are executed on the same go-routine as the call to `Set`.
func (d *DynFloat64Value) WithValidator(validator func(float64) error) *DynFloat64Value {
	d.validator = validator
	return d
}

// WithNotifier adds a function is called every time a new value is successfully set.
// Each notifier is executed in a new go-routine.
func (d *DynFloat64Value) WithNotifier(notifier func(oldValue float64, newValue float64)) *DynFloat64Value {
	d.notifier = notifier
	return d
}

// String returns the canonical string representation of the type.
func (d *DynFloat64Value) String() string {
	return fmt.Sprintf("%v", d.Get())
}

// ValidateDynFloat64Range returns a validator that checks if the float value is in range.
func ValidateDynFloat64Range(fromInclusive float64, toInclusive float64) func(float64) error {
	return func(value float64) error {
		if value > toInclusive || value < fromInclusive {
			return fmt.Errorf("value %v not in [%v, %v] range", value, fromInclusive, toInclusive)
		}
		return nil
	}
}
