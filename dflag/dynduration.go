// Copyright 2015 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package dflag

import (
	"flag"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

// DynDuration creates a `Flag` that represents `time.Duration` which is safe to change dynamically at runtime.
func DynDuration(flagSet *flag.FlagSet, name string, value time.Duration, usage string) *DynDurationValue {
	dynValue := &DynDurationValue{ptr: (*int64)(&value)}
	flagSet.Var(dynValue, name, usage)
	return dynValue
}

// DynDurationValue is a flag-related `time.Duration` value wrapper.
type DynDurationValue struct {
	DynamicFlagValueTag
	ptr       *int64
	validator func(time.Duration) error
	notifier  func(oldValue time.Duration, newValue time.Duration)
}

// Get retrieves the value in a thread-safe manner.
func (d *DynDurationValue) Get() time.Duration {
	if d.ptr == nil {
		return (time.Duration)(0)
	}
	return (time.Duration)(atomic.LoadInt64(d.ptr))
}

// Set updates the value from a string representation in a thread-safe manner.
// This operation may return an error if the provided `input` doesn't parse, or the resulting value doesn't pass an
// optional validator.
// If a notifier is set on the value, it will be invoked in a separate go-routine.
func (d *DynDurationValue) Set(input string) error {
	v, err := time.ParseDuration(strings.TrimSpace(input))
	if err != nil {
		return err
	}
	if d.validator != nil {
		if err := d.validator(v); err != nil {
			return err
		}
	}
	oldPtr := atomic.SwapInt64(d.ptr, (int64)(v))
	if d.notifier != nil {
		go d.notifier((time.Duration)(oldPtr), v)
	}
	return nil
}

// WithValidator adds a function that checks values before they're set.
// Any error returned by the validator will lead to the value being rejected.
// Validators are executed on the same go-routine as the call to `Set`.
func (d *DynDurationValue) WithValidator(validator func(time.Duration) error) *DynDurationValue {
	d.validator = validator
	return d
}

// WithNotifier adds a function is called every time a new value is successfully set.
// Each notifier is executed in a new go-routine.
func (d *DynDurationValue) WithNotifier(notifier func(oldValue time.Duration, newValue time.Duration)) *DynDurationValue {
	d.notifier = notifier
	return d
}

// String represents the canonical representation of the type.
func (d *DynDurationValue) String() string {
	return fmt.Sprintf("%v", d.Get())
}
