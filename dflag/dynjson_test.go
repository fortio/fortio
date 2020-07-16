// Copyright 2015 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package dflag

import (
	"fmt"
	"testing"
	"time"

	"flag"

	"github.com/stretchr/testify/assert"
)

var (
	defaultJSON = &outerJSON{
		FieldInts:   []int{1, 3, 3, 7},
		FieldString: "non-empty",
		FieldInner: &innerJSON{
			FieldBool: true,
		},
	}
)

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

	assert.NoError(t, set.Set("some_json_1", `{"ints": [42], "string":"bar"}`), "no error from validator when inputo k")
	assert.Error(t, set.Set("some_json_1", `{"ints": [42]}`), "error from validator when value out of range")
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
	set.Set("some_json_1", `{"ints": [42], "string":"bar"}`)
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
