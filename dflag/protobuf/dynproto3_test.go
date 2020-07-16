// Copyright 2015 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package protoflagz

import (
	"testing"

	"fmt"
	"time"

	"flag"

	"github.com/golang/protobuf/proto"
	"github.com/ldemailly/go-flagz"
	mwitkow_testproto "github.com/ldemailly/go-flagz/protobuf/testdata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	defaultProto3 = &mwitkow_testproto.SomeMsg{
		SomeString: "somevalue",
		SomeEnum:   mwitkow_testproto.SomeEnum_OPT_1,
		SomeMap:    map[string]int32{"one": 1},
	}

	someProto3JsonPbValue     = `{"someString": "wolololo", "someEnum": "OPT_2", "someMap": { "foo": 1337 }}`
	someProto3JsonPbOrigValue = `{"some_string": "wolololo", "some_enum": "OPT_2", "some_map": { "foo": 1337 }}`
	someProto3Proto           = []byte{10, 8, 119, 111, 108, 111, 108, 111, 108, 111, 16, 1, 26, 8, 10, 3, 102, 111, 111, 16, 185, 10}
	someProto3Expected        = &mwitkow_testproto.SomeMsg{
		SomeString: "wolololo",
		SomeEnum:   mwitkow_testproto.SomeEnum_OPT_2,
		SomeMap:    map[string]int32{"foo": 1337},
	}
)

func TestDynProto3_SetJSONPBAndGet(t *testing.T) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	dynFlag := DynProto3(set, "some_proto3_1", defaultProto3, "Use it or lose it")

	assert.EqualValues(t, defaultProto3, dynFlag.Get(), "value must be default after create")

	err := set.Set("some_proto3_1", someProto3JsonPbValue)
	assert.NoError(t, err, "setting value using JSONPB names must succeed")
	assert.EqualValues(t, someProto3Expected, dynFlag.Get(), "value must be set after update")

	err = set.Set("some_proto3_1", someProto3JsonPbOrigValue)
	assert.NoError(t, err, "setting value using original field namesmust succeed")
	assert.EqualValues(t, someProto3Expected, dynFlag.Get(), "value must be set after update")
}

func TestDynProto3_SetJsonPBOrigNameAndGet(t *testing.T) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	dynFlag := DynProto3(set, "some_proto3_1", defaultProto3, "Use it or lose it")

	assert.EqualValues(t, defaultProto3, dynFlag.Get(), "value must be default after create")

	err := set.Set("some_proto3_1", someProto3JsonPbOrigValue)
	assert.NoError(t, err, "setting value using original field namesmust succeed")
	assert.EqualValues(t, someProto3Expected, dynFlag.Get(), "value must be set after update")
}

func TestDynProto3_SetProtoAndGet(t *testing.T) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	dynFlag := DynProto3(set, "some_proto3_1", defaultProto3, "Use it or lose it")
	assert.EqualValues(t, defaultProto3, dynFlag.Get(), "value must be default after create")

	//t.Logf("In test: %v", string(someProto3Proto))
	something := &mwitkow_testproto.SomeMsg{}
	require.NoError(t, proto.Unmarshal([]byte(string(someProto3Proto)), something), "must succeed in normal decomp")

	err := set.Set("some_proto3_1", string(someProto3Proto))
	assert.NoError(t, err, "setting value using proto3 binary encoding must succeed")
	assert.EqualValues(t, someProto3Expected, dynFlag.Get(), "value must be set after update")
}

func TestDynProto3_IsMarkedDynamic(t *testing.T) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	DynProto3(set, "some_proto3_1", defaultProto3, "Use it or lose it")
	assert.True(t, flagz.IsFlagDynamic(set.Lookup("some_proto3_1")))
}

func TestDynProto3_FiresValidators(t *testing.T) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)

	validator := func(val proto.Message) error {
		j, ok := val.(*mwitkow_testproto.SomeMsg)
		if !ok {
			return fmt.Errorf("Bad type: %T", val)
		}
		if j.SomeString == "" {
			return fmt.Errorf("SomeString must not be empty")
		}
		return nil
	}

	DynProto3(set, "some_proto3_1", defaultProto3, "Use it or lose it").WithValidator(validator)

	assert.NoError(t, set.Set("some_proto3_1", someProto3JsonPbValue), "no error from validator when inputo k")
	assert.Error(t, set.Set("some_proto3_1", `{}`), "error from validator when value out of range")
}

func TestDynProto3_FiresNotifier(t *testing.T) {
	waitCh := make(chan bool, 1)
	notifier := func(oldVal proto.Message, newVal proto.Message) {
		assert.EqualValues(t, defaultProto3, oldVal, "old value in notify must match previous value")
		assert.EqualValues(t, someProto3Expected, newVal, "new value in notify must match set value")
		waitCh <- true
	}

	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	DynProto3(set, "some_proto3_1", defaultProto3, "Use it or lose it").WithNotifier(notifier)
	set.Set("some_proto3_1", someProto3JsonPbValue)
	select {
	case <-time.After(5 * time.Millisecond):
		assert.Fail(t, "failed to trigger notifier")
	case <-waitCh:
	}
}
