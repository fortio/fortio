// Copyright (c) Improbable Worlds Ltd, All Rights Reserved
// See LICENSE for licensing terms.

package flagz

import (
	"fmt"
	"testing"
	"time"

	flag "github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
)

func TestDynBool_SetAndGet(t *testing.T) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	dynFlag := DynBool(set, "some_bool_1", true, "Use it or lose it")
	assert.Equal(t, true, dynFlag.Get(), "value must be default after create")
	err := set.Set("some_bool_1", "false")
	assert.NoError(t, err, "setting value must succeed")
	assert.Equal(t, false, dynFlag.Get(), "value must be set after update")
}

func TestDynBool_IsMarkedDynamic(t *testing.T) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	DynBool(set, "some_bool_1", true, "Use it or lose it")
	assert.True(t, IsFlagDynamic(set.Lookup("some_bool_1")))
}

func TestDynBool_FiresValidators(t *testing.T) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	DynBool(set, "some_bool_1", true, "Use it or lose it").WithValidator(func(b bool) error {
		if b {
			return nil
		}
		return fmt.Errorf("not true!")
	})
	assert.Error(t, set.Set("some_bool_1", "false"), "error from validator when value does not satisfy validator")
}

func TestDynBool_FiresNotifier(t *testing.T) {
	waitCh := make(chan bool, 1)
	notifier := func(oldVal bool, newVal bool) {
		assert.EqualValues(t, true, oldVal, "old value in notify must match previous value")
		assert.EqualValues(t, false, newVal, "new value in notify must match set value")
		waitCh <- true
	}

	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	DynBool(set, "some_bool_1", true, "Use it or lose it").WithNotifier(notifier)
	set.Set("some_bool_1", "false")
	select {
	case <-time.After(5 * time.Millisecond):
		assert.Fail(t, "failed to trigger notifier")
	case <-waitCh:
	}
}

func Benchmark_Bool_Dyn_Get(b *testing.B) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	value := DynBool(set, "some_bool_1", true, "Use it or lose it")
	set.Set("some_bool_1", "false")
	for i := 0; i < b.N; i++ {
		x := value.Get()
		x = !x
	}
}

func Benchmark_Bool_Normal_Get(b *testing.B) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	valPtr := set.Bool("some_bool_1", true, "Use it or lose it")
	set.Set("some_bool_1", "false")
	for i := 0; i < b.N; i++ {
		x := *valPtr
		x = !x
	}
}