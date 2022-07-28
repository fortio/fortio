// Copyright 2015 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package dflag

import (
	"flag"
	"testing"
	"time"
)

func TestDynStringSlice_SetAndGet(t *testing.T) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	dynFlag := DynStringSlice(set, "some_stringslice_1", []string{"foo", "bar"}, "Use it or lose it")
	assert.Equal(t, []string{"foo", "bar"}, dynFlag.Get(), "value must be default after create")
	err := set.Set("some_stringslice_1", "car,bar")
	assert.NoError(t, err, "setting value must succeed")
	assert.Equal(t, []string{"car", "bar"}, dynFlag.Get(), "value must be set after update")
}

func TestDynStringSlice_IsMarkedDynamic(t *testing.T) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	DynStringSlice(set, "some_stringslice_1", []string{"foo", "bar"}, "Use it or lose it")
	assert.True(t, IsFlagDynamic(set.Lookup("some_stringslice_1")))
}

func TestDynStringSlice_FiresValidators(t *testing.T) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	DynStringSlice(set, "some_stringslice_1", []string{"foo", "bar"},
		"Use it or lose it").WithValidator(ValidateDynStringSliceMinElements(2))

	assert.NoError(t, set.Set("some_stringslice_1", "car,far"), "no error from validator when in range")
	assert.Error(t, set.Set("some_stringslice_1", "car"), "error from validator when value out of range")
}

func TestDynStringSlice_FiresNotifier(t *testing.T) {
	waitCh := make(chan bool, 1)
	notifier := func(oldVal []string, newVal []string) {
		assert.EqualValues(t, []string{"foo", "bar"}, oldVal, "old value in notify must match previous value")
		assert.EqualValues(t, []string{"car", "far"}, newVal, "new value in notify must match set value")
		waitCh <- true
	}

	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	DynStringSlice(set, "some_stringslice_1", []string{"foo", "bar"}, "Use it or lose it").WithNotifier(notifier)
	set.Set("some_stringslice_1", "car,far")
	select {
	case <-time.After(notifierTimeout):
		assert.Fail(t, "failed to trigger notifier")
	case <-waitCh:
	}
}
