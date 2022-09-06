// Copyright 2015 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package dflag

import (
	"flag"
	"testing"
	"time"

	"fortio.org/assert"
)

func TestDynStringSet_SetAndGet(t *testing.T) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	dynFlag := DynStringSet(set, "some_stringslice_1", []string{"foo", "bar"}, "Use it or lose it")
	assert.Equal(t, Set[string]{"foo": {}, "bar": {}}, dynFlag.Get(), "value must be default after create")
	err := set.Set("some_stringslice_1", "car,bar")
	assert.NoError(t, err, "setting value must succeed")
	assert.Equal(t, Set[string]{"car": {}, "bar": {}}, dynFlag.Get(), "value must be set after update")
}

func TestDynStringSet_Contains(t *testing.T) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	dynFlag := DynStringSet(set, "some_stringslice_1", []string{"foo", "bar"}, "Use it or lose it")
	assert.True(t, dynFlag.Contains("foo"), "contains should return true for an added value")
	assert.True(t, dynFlag.Contains("bar"), "contains should return true for an added value")
	assert.False(t, dynFlag.Contains("car"), "contains should return false for a missing value")
}

func TestDynStringSet_IsMarkedDynamic(t *testing.T) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	DynStringSet(set, "some_stringslice_1", []string{"foo", "bar"}, "Use it or lose it")
	assert.True(t, IsFlagDynamic(set.Lookup("some_stringslice_1")))
}

func TestDynStringSet_FiresValidators(t *testing.T) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	DynStringSet(set, "some_stringslice_1", []string{"foo", "bar"},
		"Use it or lose it").WithValidator(ValidateDynStringSetMinElements(2))

	assert.NoError(t, set.Set("some_stringslice_1", "car,far"), "no error from validator when in range")
	assert.Error(t, set.Set("some_stringslice_1", "car"), "error from validator when value out of range")
}

func TestDynStringSet_FiresNotifier(t *testing.T) {
	waitCh := make(chan struct{}, 1)
	notifier := func(oldVal Set[string], newVal Set[string]) {
		assert.EqualValues(t, Set[string]{"foo": {}, "bar": {}}, oldVal, "old value in notify must match previous value")
		assert.EqualValues(t, Set[string]{"car": {}, "far": {}}, newVal, "new value in notify must match set value")
		waitCh <- struct{}{}
	}

	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	DynStringSet(set, "some_stringslice_1", []string{"foo", "bar"}, "Use it or lose it").WithNotifier(notifier)
	set.Set("some_stringslice_1", "car,far")
	select {
	case <-time.After(notifierTimeout):
		assert.Fail(t, "failed to trigger notifier")
	case <-waitCh:
	}
}
