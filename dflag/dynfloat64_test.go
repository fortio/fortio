// Copyright 2015 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package flagz

import (
	"testing"

	"flag"

	"time"

	"github.com/stretchr/testify/assert"
)

func TestDynFloat64_SetAndGet(t *testing.T) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	dynFlag := DynFloat64(set, "some_float_1", 13.37, "Use it or lose it")
	assert.Equal(t, float64(13.37), dynFlag.Get(), "value must be default after create")
	err := set.Set("some_float_1", "1.337\n")
	assert.NoError(t, err, "setting value must succeed")
	assert.Equal(t, float64(1.337), dynFlag.Get(), "value must be set after update")
}

func TestDynFloat64_IsMarkedDynamic(t *testing.T) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	DynFloat64(set, "some_float_1", 13.37, "Use it or lose it")
	assert.True(t, IsFlagDynamic(set.Lookup("some_float_1")))
}

func TestDynFloat64_FiresValidators(t *testing.T) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	DynFloat64(set, "some_float_1", 13.37, "Use it or lose it").WithValidator(ValidateDynFloat64Range(10.0, 14.0))

	assert.NoError(t, set.Set("some_float_1", "13.41"), "no error from validator when in range")
	assert.Error(t, set.Set("some_float_1", "14.001"), "error from validator when value out of range")
}

func TestDynFloat64_FiresNotifier(t *testing.T) {
	waitCh := make(chan bool, 1)
	notifier := func(oldVal float64, newVal float64) {
		assert.EqualValues(t, 13.37, oldVal, "old value in notify must match previous value")
		assert.EqualValues(t, 7.11, newVal, "new value in notify must match set value")
		waitCh <- true
	}

	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	DynFloat64(set, "some_float_1", 13.37, "Use it or lose it").WithNotifier(notifier)
	set.Set("some_float_1", "7.11")
	select {
	case <-time.After(5 * time.Millisecond):
		assert.Fail(t, "failed to trigger notifier")
	case <-waitCh:
	}
}

func Benchmark_Float64_Dyn_Get(b *testing.B) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	value := DynFloat64(set, "some_float_1", 13.37, "Use it or lose it")
	set.Set("some_float_1", "14.00")
	for i := 0; i < b.N; i++ {
		x := value.Get()
		x = x + 1
	}
}

func Benchmark_Float64_Normal_Get(b *testing.B) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	valPtr := set.Float64("some_float_1", 13.37, "Use it or lose it")
	set.Set("some_float_1", "14.00")
	for i := 0; i < b.N; i++ {
		x := *valPtr
		x = x + 0.01
	}
}
