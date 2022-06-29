// Copyright 2015 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package dflag

import (
	"flag"
	"regexp"
	"testing"
	"time"
)

func TestDynString_SetAndGet(t *testing.T) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	dynFlag := DynString(set, "some_string_1", "something", "Use it or lose it")
	assert.Equal(t, "something", dynFlag.Get(), "value must be default after create")
	err := set.Set("some_string_1", "else")
	assert.NoError(t, err, "setting value must succeed")
	assert.Equal(t, "else", dynFlag.Get(), "value must be set after update")
}

func TestDynString_InputMutator(t *testing.T) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	dynFlagA := DynString(set, "some_string_a", "something", "Use it or lose it")
	dynFlagB := DynString(set, "some_string_b", "something", "Use it or lose it").WithInputMutator(nil)
	withSpaces := " \na\nb\n\n"
	err := set.Set("some_string_a", withSpaces)
	assert.NoError(t, err, "setting value must succeed")
	assert.Equal(t, "a\nb", dynFlagA.Get(), "value should be trimmed by default")
	err = set.Set("some_string_b", withSpaces)
	assert.NoError(t, err, "setting value must succeed")
	assert.Equal(t, withSpaces, dynFlagB.Get(), "value should be verbatim when removing WithInputMutator")
	dynFlagB.WithInputMutator(func(inp string) string { return "X" + inp + "Y" })
	err = set.Set("some_string_b", withSpaces)
	assert.NoError(t, err, "setting value must succeed")
	assert.Equal(t, "X"+withSpaces+"Y", dynFlagB.Get(), "value should be changed with X...Y WithInputMutator")
	assert.Equal(t, "dyn_string", dynFlagA.Type(), "type name should be dyn_string")
}

func TestDynString_IsMarkedDynamic(t *testing.T) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	DynString(set, "some_string_1", "something", "Use it or lose it")
	assert.True(t, IsFlagDynamic(set.Lookup("some_string_1")))
}

func TestDynString_FiresValidators(t *testing.T) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	regex := regexp.MustCompile("^[a-z]{2,8}$")
	DynString(set, "some_string_1", "something", "Use it or lose it").WithValidator(ValidateDynStringMatchesRegex(regex))

	assert.NoError(t, set.Set("some_string_1", "else"), "no error from validator when in range")
	assert.Error(t, set.Set("some_string_1", "else1"), "error from validator when value out of range")
}

func TestDynString_FiresNotifier(t *testing.T) {
	waitCh := make(chan bool, 1)
	notifier := func(oldVal string, newVal string) {
		assert.EqualValues(t, "something", oldVal, "old value in notify must match previous value")
		assert.EqualValues(t, "somethingelse", newVal, "new value in notify must match set value")
		waitCh <- true
	}

	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	DynString(set, "some_string_1", "something", "Use it or lose it").WithNotifier(notifier)
	set.Set("some_string_1", "somethingelse")
	select {
	case <-time.After(5 * time.Millisecond):
		assert.Fail(t, "failed to trigger notifier")
	case <-waitCh:
	}
}

func Benchmark_String_Dyn_Get(b *testing.B) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	value := DynString(set, "some_string_1", "something", "Use it or lose it")
	set.Set("some_string_1", "else")
	for i := 0; i < b.N; i++ {
		x := value.Get()
		x += "foo" // nolint: ineffassign
	}
}

func Benchmark_String_Normal_get(b *testing.B) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	valPtr := set.String("some_string_1", "something", "Use it or lose it")
	set.Set("some_string_1", "else")
	for i := 0; i < b.N; i++ {
		x := *valPtr
		x += "foo" // nolint: ineffassign
	}
}
