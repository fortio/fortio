// Copyright 2015 Michal Witkowski.
// Copyright 2022 Fortio Authors.
// All Rights Reserved.
// See LICENSE for licensing terms.

package dflag

import (
	"encoding/json"
	"flag"
	"reflect"
)

// JSON is the only/most kudlgy type, not playing so well or reusing as much as the rest of the generic re-implementation.

// DynJSON creates a `Flag` that is backed by an arbitrary JSON which is safe to change dynamically at runtime.
// The `value` must be a pointer to a struct that is JSON (un)marshallable.
// New values based on the default constructor of `value` type will be created on each update.
func DynJSON(flagSet *flag.FlagSet, name string, value interface{}, usage string) *DynJSONValue {
	reflectVal := reflect.ValueOf(value)

	if reflectVal.Kind() != reflect.Ptr ||
		(reflectVal.Elem().Kind() != reflect.Struct && reflectVal.Elem().Kind() != reflect.Slice) {
		panic("DynJSON value must be a pointer to a struct or to a slice")
	}
	dynValue := DynJSONValue{}
	dynInit(&dynValue.DynValue, flagSet, name, value, usage)
	dynValue.structType = reflectVal.Type().Elem()
	flagSet.Var(&dynValue, name, usage) // use our Set()
	flagSet.Lookup(name).DefValue = dynValue.usageString()
	return &dynValue
}

// DynJSONValue is a flag-related JSON struct value wrapper.
type DynJSONValue struct {
	DynValue[interface{}]
	structType reflect.Type
}

// IsJSON always return true (method is present for the DynamicJSONFlagValue interface tagging).
func (d *DynJSONValue) IsJSON() bool {
	return true
}

// Set updates the value from a string representation in a thread-safe manner.
// This operation may return an error if the provided `input` doesn't parse, or the resulting value doesn't pass an
// optional validator.
// If a notifier is set on the value, it will be invoked in a separate go-routine.
func (d *DynJSONValue) Set(rawInput string) error {
	input := rawInput
	if d.inpMutator != nil {
		input = d.inpMutator(rawInput)
	}
	val := reflect.New(d.structType).Interface()
	if err := json.Unmarshal([]byte(input), val); err != nil {
		return err
	}
	return d.PostSet(val)
}

// String returns the canonical string representation of the type.
func (d *DynJSONValue) String() string {
	if !d.ready {
		return ""
	}
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
