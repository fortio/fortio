// Copyright 2015 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package watcher_test

import (
	"os"
	"testing"
	"time"

	"flag"

	watcher "github.com/ldemailly/go-flagz/watcher"
	etcd_harness "github.com/mwitkow/go-etcd-harness"

	etcd "github.com/coreos/etcd/client"
	"github.com/ldemailly/go-flagz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"golang.org/x/net/context"
)

const (
	prefix = "/updater_test/"
)

// Define the suite, and absorb the built-in basic suite
// functionality from testify - including assertion methods.
type watcherTestSuite struct {
	suite.Suite
	keys etcd.KeysAPI

	flagSet *flag.FlagSet
	watcher *watcher.Watcher
}

// Clean up the etcd state before each test.
func (s *watcherTestSuite) SetupTest() {
	s.keys.Delete(newCtx(), prefix, &etcd.DeleteOptions{Dir: true, Recursive: true})
	_, err := s.keys.Set(newCtx(), prefix, "", &etcd.SetOptions{Dir: true})
	if err != nil {
		s.T().Fatalf("cannot create empty dir %v: %v", prefix, err)
	}
	s.flagSet = flag.NewFlagSet("updater_test", flag.ContinueOnError)
	s.watcher, err = watcher.New(s.flagSet, s.keys, prefix, &testingLog{T: s.T()})
	if err != nil {
		s.T().Fatalf("cannot create updater: %v", err)
	}
}

func (s *watcherTestSuite) setFlagzValue(flagzName string, value string) {
	_, err := s.keys.Set(newCtx(), prefix+flagzName, value, &etcd.SetOptions{})
	if err != nil {
		s.T().Fatalf("failed setting flagz value: %v", err)
	}
	s.T().Logf("test has set flag=%v to value %v", flagzName, value)
}

func (s *watcherTestSuite) getFlagzValue(flagzName string) string {
	resp, err := s.keys.Get(newCtx(), prefix+flagzName, &etcd.GetOptions{})
	if err != nil {
		s.T().Logf("failed getting flagz value: %v", err)
		return ""
	}
	return resp.Node.Value
}

// Tear down the updater
func (s *watcherTestSuite) TearDownTest() {
	s.watcher.Stop()
	time.Sleep(100 * time.Millisecond)
}

func (s *watcherTestSuite) Test_ErrorsOnInitialUnknownFlag() {
	flagz.DynInt64(s.flagSet, "someint", 1337, "some int usage")
	s.setFlagzValue("anotherint", "999")
	s.Require().Error(s.watcher.Initialize(), "initialize should complain about unknown flag")
}

func (s *watcherTestSuite) Test_SetsInitialValues() {
	someInt := flagz.DynInt64(s.flagSet, "someint", 1337, "some int usage")
	someString := flagz.DynString(s.flagSet, "somestring", "initial_value", "some int usage")
	anotherString := flagz.DynString(s.flagSet, "anotherstring", "default_value", "some int usage")
	normalString := s.flagSet.String("normalstring", "default_value", "some int usage")

	s.setFlagzValue("someint", "2015")
	s.setFlagzValue("somestring", "changed_value")
	s.setFlagzValue("normalstring", "changed_value2")

	require.NoError(s.T(), s.watcher.Initialize())

	assert.Equal(s.T(), int64(2015), someInt.Get(), "int flag should change value")
	assert.Equal(s.T(), "changed_value", someString.Get(), "string flag should change value")
	assert.Equal(s.T(), "default_value", anotherString.Get(), "anotherstring should be unchanged")
	assert.Equal(s.T(), "changed_value2", *normalString, "anotherstring should be unchanged")

}

func (s *watcherTestSuite) Test_DynamicUpdate() {
	someInt := flagz.DynInt64(s.flagSet, "someint", 1337, "some int usage")
	require.NoError(s.T(), s.watcher.Initialize())
	require.NoError(s.T(), s.watcher.Start())
	require.Equal(s.T(), int64(1337), someInt.Get(), "int flag should not change value")
	s.setFlagzValue("someint", "2014")
	eventually(s.T(), 1*time.Second,
		assert.ObjectsAreEqualValues, int64(2014),
		func() interface{} { return someInt.Get() },
		"someint value should change to 2014")
	s.setFlagzValue("someint", "2015")
	eventually(s.T(), 1*time.Second,
		assert.ObjectsAreEqualValues, 2015,
		func() interface{} { return someInt.Get() },
		"someint value should change to 2015")
	s.setFlagzValue("someint", "2016")
	eventually(s.T(), 1*time.Second,
		assert.ObjectsAreEqualValues, int64(2016),
		func() interface{} { return someInt.Get() },
		"someint value should change to 2016")
}

func (s *watcherTestSuite) Test_DynamicUpdateRestoresGoodState() {
	someInt := flagz.DynInt64(s.flagSet, "someint", 1337, "some int usage")
	someFloat := flagz.DynFloat64(s.flagSet, "somefloat", 1.337, "some int usage")
	s.setFlagzValue("someint", "2015")
	require.NoError(s.T(), s.watcher.Initialize())
	require.NoError(s.T(), s.watcher.Start())
	require.EqualValues(s.T(), 2015, someInt.Get(), "int flag should change value")
	require.EqualValues(s.T(), 1.337, someFloat.Get(), "float flag should not change value")

	// Bad update causing a rollback.
	s.setFlagzValue("someint", "randombleh")
	eventually(s.T(), 1*time.Second,
		assert.ObjectsAreEqualValues,
		"2015",
		func() interface{} {
			return s.getFlagzValue("someint")
		},
		"someint failure should revert etcd value to 2015")

	// Make sure we can continue updating.
	s.setFlagzValue("someint", "2016")
	s.setFlagzValue("somefloat", "3.14")
	eventually(s.T(), 1*time.Second,
		assert.ObjectsAreEqualValues, int64(2016),
		func() interface{} { return someInt.Get() },
		"someint value should change, after rolled back")
	eventually(s.T(), 1*time.Second,
		assert.ObjectsAreEqualValues, float64(3.14),
		func() interface{} { return someFloat.Get() },
		"somefloat value should change")

}

func (s *watcherTestSuite) Test_DynamicUpdate_WroteBadSubdirectory() {
	someInt := flagz.DynInt64(s.flagSet, "someint", 1337, "some int usage")
	require.NoError(s.T(), s.watcher.Initialize())
	require.NoError(s.T(), s.watcher.Start())

	s.setFlagzValue("subdir1/subdir2/leaf", "randombleh")
	eventually(s.T(), 1*time.Second, assert.ObjectsAreEqualValues, nil,
		func() interface{} {
			_, err := s.keys.Get(newCtx(), prefix+"subdir1/subdir2/leaf", &etcd.GetOptions{})
			return err
		},
		"mistaken subdirectories are left in tact")

	s.setFlagzValue("someint", "7331")
	eventually(s.T(), 1*time.Second, assert.ObjectsAreEqualValues, 7331,
		func() interface{} { return someInt.Get() },
		"writing a bad directory shouldn't inhibit the watcher")
}

func (s *watcherTestSuite) Test_DynamicUpdate_DoesntUpdateNonDynamicFlags() {
	someInt := flagz.DynInt64(s.flagSet, "someint", 1337, "some int usage")
	someString := s.flagSet.String("somestring", "initial_value", "some int usage")

	require.NoError(s.T(), s.watcher.Initialize())
	require.NoError(s.T(), s.watcher.Start())

	// This write must not make it to someString until another .Initialize is called.
	s.setFlagzValue("somestring", "newvalue")

	s.setFlagzValue("someint", "7331")
	eventually(s.T(), 1*time.Second, assert.ObjectsAreEqualValues, 7331,
		func() interface{} { return someInt.Get() },
		"the dynamic someint write that acts as a barrier, must succeed")
	assert.EqualValues(s.T(), "initial_value", *someString, "somestring must not be overwritten dynamically")

	eventually(s.T(), 1*time.Second, assert.ObjectsAreEqualValues, "newvalue",
		func() interface{} { return s.getFlagzValue("somestring") },
		"the non-dynamic somestring shouldnt affect the values in etcd")
}

func TestUpdaterSuite(t *testing.T) {
	// Disable test until https://github.com/mwitkow/go-etcd-harness/issues/1 is fixed
	t.Skip("go-etcd-hardness not working for now")
	harness, err := etcd_harness.New(os.Stderr)
	if err != nil {
		t.Fatalf("failed starting test server: %v", err)
	}
	t.Logf("will use etcd test endpoint: %v", harness.Endpoint)
	defer func() {
		harness.Stop()
		t.Logf("cleaned up etcd test server")
	}()
	suite.Run(t, &watcherTestSuite{keys: etcd.NewKeysAPI(harness.Client)})
}

type assertFunc func(expected, actual interface{}) bool
type getter func() interface{}

// eventually tries a given Assert function 5 times over the period of time.
func eventually(t *testing.T, duration time.Duration,
	af assertFunc, expected interface{}, actual getter, msgFmt string, msgArgs ...interface{}) {
	increment := duration / 5
	for i := 0; i < 5; i++ {
		time.Sleep(increment)
		if af(expected, actual()) {
			return
		}
	}
	t.Fatalf(msgFmt, msgArgs...)
}

func newCtx() context.Context {
	c, _ := context.WithTimeout(context.TODO(), 500*time.Millisecond)
	return c
}

// Abstraction that allows us to pass the *testing.T as a logger to the updater.
type testingLog struct {
	T *testing.T
}

func (tl *testingLog) Printf(format string, v ...interface{}) {
	tl.T.Logf(format+"\n", v...)
}
