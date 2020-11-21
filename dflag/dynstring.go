// Copyright 2015 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package dflag

import (
	"flag"
	"fmt"
	"regexp"
	"sync/atomic"
	"unsafe"
)

// DynString creates a `Flag` that represents `string` which is safe to change dynamically at runtime.
func DynString(flagSet *flag.FlagSet, name string, value string, usage string) *DynStringValue {
	dynValue := &DynStringValue{ptr: unsafe.Pointer(&value)}
	flagSet.Var(dynValue, name, usage)
	return dynValue
}

// DynStringValue is a flag-related `time.Duration` value wrapper.
type DynStringValue struct {
	DynamicFlagValueTag
	ptr          unsafe.Pointer
	validator    func(string) error
	notifier     func(oldValue string, newValue string)
	syncNotifier bool
}

// Get retrieves the value in a thread-safe manner.
func (d *DynStringValue) Get() string {
	if d.ptr == nil {
		return ""
	}
	ptr := atomic.LoadPointer(&d.ptr)
	return *(*string)(ptr)
}

// Set updates the value from a string representation in a thread-safe manner.
// This operation may return an error if the provided `input` doesn't parse, or the resulting value doesn't pass an
// optional validator.
// If a notifier is set on the value, it will be invoked in a separate go-routine.
func (d *DynStringValue) Set(val string) error {
	if d.validator != nil {
		if err := d.validator(val); err != nil {
			return err
		}
	}
	oldPtr := atomic.SwapPointer(&d.ptr, unsafe.Pointer(&val))
	if d.notifier != nil {
		if d.syncNotifier {
			d.notifier(*(*string)(oldPtr), val)
		} else {
			go d.notifier(*(*string)(oldPtr), val)
		}
	}
	return nil
}

// WithValidator adds a function that checks values before they're set.
// Any error returned by the validator will lead to the value being rejected.
// Validators are executed on the same go-routine as the call to `Set`.
func (d *DynStringValue) WithValidator(validator func(string) error) *DynStringValue {
	d.validator = validator
	return d
}

// WithNotifier adds a function is called every time a new value is successfully set.
// Each notifier is executed in a new go-routine.
func (d *DynStringValue) WithNotifier(notifier func(oldValue string, newValue string)) *DynStringValue {
	d.notifier = notifier
	return d
}

// WithSyncNotifier adds a function is called synchronously every time a new value is successfully set.
func (d *DynStringValue) WithSyncNotifier(notifier func(oldValue string, newValue string)) *DynStringValue {
	d.notifier = notifier
	d.syncNotifier = true
	return d
}

// String represents the canonical representation of the type.
func (d *DynStringValue) String() string {
	return fmt.Sprintf("%v", d.Get())
}

// ValidateDynStringMatchesRegex returns a validator function that checks all flag's values against regex.
func ValidateDynStringMatchesRegex(matcher *regexp.Regexp) func(string) error {
	return func(value string) error {
		if !matcher.MatchString(value) {
			return fmt.Errorf("value %v must match regex %v", value, matcher)
		}
		return nil
	}
}
