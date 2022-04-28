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

// DynStringSet creates a `Flag` that represents `map[string]struct{}` which is safe to change dynamically at runtime.
// Unlike `pflag.StringSlice`, consecutive sets don't append to the slice, but override it.
func DynStringSet(flagSet *flag.FlagSet, name string, value []string, usage string) *DynStringSetValue {
	set := buildStringSet(value)
	dynValue := &DynStringSetValue{ptr: unsafe.Pointer(&set)}
	flagSet.Var(dynValue, name, usage)
	return dynValue
}

// DynStringSetValue is a flag-related `map[string]struct{}` value wrapper.
type DynStringSetValue struct {
	DynamicFlagValueTag
	ptr       unsafe.Pointer
	validator func(map[string]struct{}) error
	notifier  func(oldValue map[string]struct{}, newValue map[string]struct{})
}

// Get retrieves the value in a thread-safe manner.
func (d *DynStringSetValue) Get() map[string]struct{} {
	if d.ptr == nil {
		return make(map[string]struct{})
	}
	p := (*map[string]struct{})(atomic.LoadPointer(&d.ptr))
	return *p
}

// Set updates the value from a string representation in a thread-safe manner.
// This operation may return an error if the provided `input` doesn't parse, or the resulting value doesn't pass an
// optional validator.
// If a notifier is set on the value, it will be invoked in a separate go-routine.
func (d *DynStringSetValue) Set(val string) error {
	v, err := csv.NewReader(strings.NewReader(val)).Read()
	if err != nil {
		return err
	}
	s := buildStringSet(v)
	if d.validator != nil {
		if err := d.validator(s); err != nil {
			return err
		}
	}
	oldPtr := atomic.SwapPointer(&d.ptr, unsafe.Pointer(&s))
	if d.notifier != nil {
		go d.notifier(*(*map[string]struct{})(oldPtr), s)
	}
	return nil
}

// Contains returns whether the specified string is in the flag.
func (d *DynStringSetValue) Contains(val string) bool {
	v := d.Get()
	_, ok := v[val]
	return ok
}

// WithValidator adds a function that checks values before they're set.
// Any error returned by the validator will lead to the value being rejected.
// Validators are executed on the same go-routine as the call to `Set`.
func (d *DynStringSetValue) WithValidator(validator func(map[string]struct{}) error) *DynStringSetValue {
	d.validator = validator
	return d
}

// WithNotifier adds a function that is called every time a new value is successfully set.
// Each notifier is executed asynchronously in a new go-routine.
func (d *DynStringSetValue) WithNotifier(notifier func(oldValue map[string]struct{},
	newValue map[string]struct{})) *DynStringSetValue {
	d.notifier = notifier
	return d
}

// String represents the canonical representation of the type.
func (d *DynStringSetValue) String() string {
	v := d.Get()
	arr := make([]string, 0, len(v))
	for k := range v {
		arr = append(arr, k)
	}
	return fmt.Sprintf("%v", arr)
}

// ValidateDynStringSetMinElements validates that the given string slice has at least x elements.
func ValidateDynStringSetMinElements(count int) func(map[string]struct{}) error {
	return func(value map[string]struct{}) error {
		if len(value) < count {
			return fmt.Errorf("value slice %v must have at least %v elements", value, count)
		}
		return nil
	}
}

func buildStringSet(items []string) map[string]struct{} {
	res := map[string]struct{}{}
	for _, item := range items {
		res[item] = struct{}{}
	}
	return res
}
