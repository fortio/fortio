// Copyright 2015 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package dflag

import (
	"flag"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var defaultJSON = &outerJSON{
	FieldInts:   []int{1, 3, 3, 7},
	FieldString: "non-empty",
	FieldInner: &innerJSON{
		FieldBool: true,
	},
}

var defaultJSONElemOne = &outerJSON{
	FieldInts:   []int{1, 3, 3, 7},
	FieldString: "non-empty",
	FieldInner: &innerJSON{
		FieldBool: true,
	},
}

var defaultJSONElemTwo = &outerJSON{
	FieldInts:   []int{2, 3, 4, 5},
	FieldString: "non-empty",
	FieldInner: &innerJSON{
		FieldBool: false,
	},
}

var defaultJSONArray = &[]outerJSON{*defaultJSONElemOne, *defaultJSONElemTwo}

func TestDynJSON_SetAndGet(t *testing.T) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	dynFlag := DynJSON(set, "some_json_1", defaultJSON, "Use it or lose it")

	assert.EqualValues(t, defaultJSON, dynFlag.Get(), "value must be default after create")

	err := set.Set("some_json_1", `{"ints": [42], "string": "new-value", "inner": { "bool": false } }`)
	assert.NoError(t, err, "setting value must succeed")
	assert.EqualValues(t,
		&outerJSON{FieldInts: []int{42}, FieldString: "new-value", FieldInner: &innerJSON{FieldBool: false}},
		dynFlag.Get(),
		"value must be set after update")
}

func TestDynJSON_IsMarkedDynamic(t *testing.T) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	DynJSON(set, "some_json_1", defaultJSON, "Use it or lose it")
	assert.True(t, IsFlagDynamic(set.Lookup("some_json_1")))
}

func TestDynJSON_FiresValidators(t *testing.T) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)

	validator := func(val interface{}) error {
		j, ok := val.(*outerJSON)
		if !ok {
			return fmt.Errorf("Bad type: %T", val)
		}
		if j.FieldString == "" {
			return fmt.Errorf("FieldString must not be empty")
		}
		return nil
	}

	DynJSON(set, "some_json_1", defaultJSON, "Use it or lose it").WithValidator(validator)

	assert.NoError(t, set.Set("some_json_1", `{"ints": [42], "string":"bar"}`),
		"no error from validator")
	assert.Error(t, set.Set("some_json_1", `{"ints": [42]}`),
		"error from validator when value out of range")
}

func TestDynJSON_FiresNotifier(t *testing.T) {
	waitCh := make(chan bool, 1)
	notifier := func(oldVal interface{}, newVal interface{}) {
		assert.EqualValues(t, defaultJSON, oldVal, "old value in notify must match previous value")
		assert.EqualValues(t, &outerJSON{FieldInts: []int{42}, FieldString: "bar"}, newVal, "new value in notify must match set value")
		waitCh <- true
	}

	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	DynJSON(set, "some_json_1", defaultJSON, "Use it or lose it").WithNotifier(notifier)
	err := set.Set("some_json_1", `{"ints": [42], "string":"bar"}`)
	assert.NoError(t, err, "setting value must succeed")

	select {
	case <-time.After(5 * time.Millisecond):
		assert.Fail(t, "failed to trigger notifier")
	case <-waitCh:
	}
}

func TestDynJSONArray_SetAndGet(t *testing.T) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	dynFlag := DynJSON(set, "some_json_array", defaultJSONArray, "Use it or lose it")

	assert.EqualValues(t, defaultJSONArray, dynFlag.Get(), "value must be default after create")

	err := set.Set("some_json_array", `[{"ints": [42], "string": "new-value", "inner": { "bool": false } }, 
																							{"ints": [24], "string": "new-value", "inner": { "bool": true } }]`)
	assert.NoError(t, err, "setting value must succeed")

	newJSONArray := &[]outerJSON{
		{FieldInts: []int{42}, FieldString: "new-value", FieldInner: &innerJSON{FieldBool: false}},
		{FieldInts: []int{24}, FieldString: "new-value", FieldInner: &innerJSON{FieldBool: true}},
	}

	assert.EqualValues(t,
		newJSONArray,
		dynFlag.Get(),
		"value must be set after update")
}

func TestDynJSONArray_IsMarkedDynamic(t *testing.T) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	DynJSON(set, "some_json_array", defaultJSONArray, "Use it or lose it")
	assert.True(t, IsFlagDynamic(set.Lookup("some_json_array")))
}

func TestDynJSONArray_FiresValidators(t *testing.T) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)

	validator := func(val interface{}) error {
		j, ok := val.(*[]outerJSON)
		if !ok {
			return fmt.Errorf("Bad type: %T", val)
		}

		for _, v := range *j {
			if v.FieldString == "" {
				return fmt.Errorf("FieldString must not be empty")
			}
		}
		return nil
	}

	DynJSON(set, "some_json_array", defaultJSONArray, "Use it or lose it").WithValidator(validator)

	assert.NoError(t, set.Set("some_json_array",
		`[{"ints": [42], "string":"bar"}, {"ints": [24], "string":"foo"}]`),
		"no error from validator when inputo k")
	assert.Error(t, set.Set("some_json_array", `{"ints": [42]}`),
		"error from validator when required value is missing")
}

func TestDynJSONArray_FiresNotifier(t *testing.T) {
	waitCh := make(chan bool, 1)
	notifier := func(oldVal interface{}, newVal interface{}) {
		assert.EqualValues(t, defaultJSONArray, oldVal, "old value in notify must match previous value")
		assert.EqualValues(t, &[]outerJSON{{
			FieldInts: []int{42}, FieldString: "new-value",
			FieldInner: &innerJSON{FieldBool: false},
		}}, newVal, "new value in notify must match set value")
		waitCh <- true
	}

	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	DynJSON(set, "some_json_array", defaultJSONArray, "Use it or lose it").WithNotifier(notifier)

	err := set.Set("some_json_array", `[{"ints": [42], "string": "new-value", "inner": { "bool": false }}]`)
	assert.NoError(t, err, "setting value must succeed")

	select {
	case <-time.After(5 * time.Millisecond):
		assert.Fail(t, "failed to trigger notifier")
	case <-waitCh:
	}
}

type outerJSON struct {
	FieldInts   []int      `json:"ints"`
	FieldString string     `json:"string"`
	FieldInner  *innerJSON `json:"inner"`
}

type innerJSON struct {
	FieldBool bool `json:"bool"`
}
