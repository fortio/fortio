// Copyright 2015 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package dflag

import (
	"encoding/json"
	"flag"
	"reflect"
	"sync/atomic"
	"unsafe"
)

// DynJSON creates a `Flag` that is backed by an arbitrary JSON which is safe to change dynamically at runtime.
// The `value` must be a pointer to a struct that is JSON (un)marshallable.
// New values based on the default constructor of `value` type will be created on each update.
func DynJSON(flagSet *flag.FlagSet, name string, value interface{}, usage string) *DynJSONValue {
	reflectVal := reflect.ValueOf(value)

	if reflectVal.Kind() != reflect.Ptr ||
		(reflectVal.Elem().Kind() != reflect.Struct && reflectVal.Elem().Kind() != reflect.Slice) {
		panic("DynJSON value must be a pointer to a struct or to a slice")
	}

	dynValue := &DynJSONValue{
		ptr:        unsafe.Pointer(reflectVal.Pointer()),
		structType: reflectVal.Type().Elem(),
		flagSet:    flagSet,
		flagName:   name,
	}
	flagSet.Var(dynValue, name, usage)
	flagSet.Lookup(name).DefValue = dynValue.usageString()
	return dynValue
}

// DynJSONValue is a flag-related JSON struct value wrapper.
type DynJSONValue struct {
	DynamicFlagValueTag
	structType reflect.Type
	ptr        unsafe.Pointer
	validator  func(interface{}) error
	notifier   func(oldValue interface{}, newValue interface{})
	flagName   string
	flagSet    *flag.FlagSet
}

// IsJSON always return true (method is present for the DynamicJSONFlagValue interface tagging).
func (d *DynJSONValue) IsJSON() bool {
	return true
}

// Get retrieves the value in its original JSON struct type in a thread-safe manner.
func (d *DynJSONValue) Get() interface{} {
	if d.ptr == nil {
		return ""
	}
	return d.unsafeToStoredType(atomic.LoadPointer(&d.ptr))
}

// Set updates the value from a string representation in a thread-safe manner.
// This operation may return an error if the provided `input` doesn't parse, or the resulting value doesn't pass an
// optional validator.
// If a notifier is set on the value, it will be invoked in a separate go-routine.
func (d *DynJSONValue) Set(input string) error {
	someStruct := reflect.New(d.structType).Interface()
	if err := json.Unmarshal([]byte(input), someStruct); err != nil {
		return err
	}
	if d.validator != nil {
		if err := d.validator(someStruct); err != nil {
			return err
		}
	}
	oldPtr := atomic.SwapPointer(&d.ptr, unsafe.Pointer(reflect.ValueOf(someStruct).Pointer()))
	if d.notifier != nil {
		go d.notifier(d.unsafeToStoredType(oldPtr), someStruct)
	}
	return nil
}

// WithValidator adds a function that checks values before they're set.
// Any error returned by the validator will lead to the value being rejected.
// Validators are executed on the same go-routine as the call to `Set`.
func (d *DynJSONValue) WithValidator(validator func(interface{}) error) *DynJSONValue {
	d.validator = validator
	return d
}

// WithNotifier adds a function is called every time a new value is successfully set.
// Each notifier is executed in a new go-routine.
func (d *DynJSONValue) WithNotifier(notifier func(oldValue interface{}, newValue interface{})) *DynJSONValue {
	d.notifier = notifier
	return d
}

// WithFileFlag adds an companion <name>_path flag that allows this value to be read from a file with dflag.ReadFileFlags.
//
// This is useful for reading large JSON files as flags. If the companion flag's value (whether default or overwritten)
// is set to empty string, nothing is read.
//
// Flag value reads are subject to notifiers and validators.
func (d *DynJSONValue) WithFileFlag(defaultPath string) (*DynJSONValue, *FileReadValue) {
	return d, FileReadFlag(d.flagSet, d.flagName, defaultPath)
}

// String returns the canonical string representation of the type.
func (d *DynJSONValue) String() string {
	out, err := json.Marshal(d.Get())
	if err != nil {
		return "ERR"
	}
	return string(out)
}

func (d *DynJSONValue) usageString() string {
	s := d.String()
	if len(s) > 128 {
		return "{ ... truncated ... }"
	}
	return s
}

func (d *DynJSONValue) unsafeToStoredType(p unsafe.Pointer) interface{} {
	n := reflect.NewAt(d.structType, p)
	return n.Interface()
}
