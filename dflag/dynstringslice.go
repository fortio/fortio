// Copyright 2015 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package dflag

import (
	"encoding/csv"
	"flag"
	"fmt"
	"strings"
	"sync/atomic"
	"unsafe"
)

// DynStringSlice creates a `Flag` that represents `[]string` which is safe to change dynamically at runtime.
// Unlike `pflag.StringSlice`, consecutive sets don't append to the slice, but override it.
func DynStringSlice(flagSet *flag.FlagSet, name string, value []string, usage string) *DynStringSliceValue {
	dynValue := &DynStringSliceValue{ptr: unsafe.Pointer(&value)}
	flagSet.Var(dynValue, name, usage)
	return dynValue
}

// DynStringSliceValue is a flag-related `time.Duration` value wrapper.
type DynStringSliceValue struct {
	DynamicFlagValueTag
	ptr       unsafe.Pointer
	validator func([]string) error
	notifier  func(oldValue []string, newValue []string)
}

// Get retrieves the value in a thread-safe manner.
func (d *DynStringSliceValue) Get() []string {
	if d.ptr == nil {
		return []string{}
	}
	p := (*[]string)(atomic.LoadPointer(&d.ptr))
	return *p
}

// Set updates the value from a string representation in a thread-safe manner.
// This operation may return an error if the provided `input` doesn't parse, or the resulting value doesn't pass an
// optional validator.
// If a notifier is set on the value, it will be invoked in a separate go-routine.
func (d *DynStringSliceValue) Set(val string) error {
	v, err := csv.NewReader(strings.NewReader(val)).Read()
	if err != nil {
		return err
	}
	if d.validator != nil {
		if err := d.validator(v); err != nil {
			return err
		}
	}
	oldPtr := atomic.SwapPointer(&d.ptr, unsafe.Pointer(&v))
	if d.notifier != nil {
		go d.notifier(*(*[]string)(oldPtr), v)
	}
	return nil
}

// WithValidator adds a function that checks values before they're set.
// Any error returned by the validator will lead to the value being rejected.
// Validators are executed on the same go-routine as the call to `Set`.
func (d *DynStringSliceValue) WithValidator(validator func([]string) error) *DynStringSliceValue {
	d.validator = validator
	return d
}

// WithNotifier adds a function that is called every time a new value is successfully set.
// Each notifier is executed asynchronously in a new go-routine.
func (d *DynStringSliceValue) WithNotifier(notifier func(oldValue []string, newValue []string)) *DynStringSliceValue {
	d.notifier = notifier
	return d
}

// String represents the canonical representation of the type.
func (d *DynStringSliceValue) String() string {
	return fmt.Sprintf("%v", d.Get())
}

// ValidateDynStringSliceMinElements validates that the given string slice has at least x elements.
func ValidateDynStringSliceMinElements(count int) func([]string) error {
	return func(value []string) error {
		if len(value) < count {
			return fmt.Errorf("value slice %v must have at least %v elements", value, count)
		}
		return nil
	}
}
