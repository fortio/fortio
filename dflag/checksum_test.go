// Copyright 2015 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package dflag_test

import (
	"flag"
	"testing"
	"time"

	"fortio.org/fortio/dflag"
)

var (
	assert  = dflag.Testify{}
	require = assert
)

func TestChecksumFlagSet_Differs(t *testing.T) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	dflag.DynDuration(set, "some_duration_1", 5*time.Second, "Use it or lose it")
	dflag.DynInt64(set, "some_int_1", 13371337, "Use it or lose it")
	set.String("static_string_1", "foobar", "meh")

	preInitChecksum := dflag.ChecksumFlagSet(set, nil)
	t.Logf("pre init checksum:  %x", preInitChecksum)

	set.Parse([]string{"--some_duration_1", "3s", "--static_string_1", "goodbar"})
	postInitChecksum := dflag.ChecksumFlagSet(set, nil)
	t.Logf("post init checksum: %x", postInitChecksum)
	assert.NotEqual(t, preInitChecksum, postInitChecksum, "checksum must be different init changed 2 flags")

	require.NoError(t, set.Set("some_int_1", "1337"))
	postSet1Checksum := dflag.ChecksumFlagSet(set, nil)
	t.Logf("post set1 checksum: %x", postSet1Checksum)
	assert.NotEqual(t, postInitChecksum, postSet1Checksum, "checksum must be different after a internal flag change")

	require.NoError(t, set.Set("some_duration_1", "4s"))
	postSet2Checksum := dflag.ChecksumFlagSet(set, nil)
	t.Logf("post set2 checksum: %x", postSet2Checksum)
	assert.NotEqual(t, postSet1Checksum, postSet2Checksum, "checksum must be different after a internal flag change")

	require.NoError(t, set.Set("some_duration_1", "3s"))
	postSet3Checksum := dflag.ChecksumFlagSet(set, nil)
	t.Logf("post set3 checksum: %x", postSet3Checksum)
	assert.EqualValues(t, postSet1Checksum, postSet3Checksum, "flipping back duration flag to state at set1 should make it equal")
}

func TestChecksumFlagSet_Filters(t *testing.T) {
	filterOnlyDuration := func(f *flag.Flag) bool { return f.Name == "some_duration_1" }
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	dflag.DynDuration(set, "some_duration_1", 5*time.Second, "Use it or lose it")
	dflag.DynInt64(set, "some_int_1", 13371337, "Use it or lose it")

	set.Parse([]string{"--some_duration_1", "3s", "--some_int_1", "1337"})
	postInitChecksum := dflag.ChecksumFlagSet(set, filterOnlyDuration)
	t.Logf("post init checksum: %x", postInitChecksum)

	require.NoError(t, set.Set("some_int_1", "1337"))
	postSet1Checksum := dflag.ChecksumFlagSet(set, filterOnlyDuration)
	t.Logf("post set1 checksum: %x", postSet1Checksum)
	assert.EqualValues(t, postInitChecksum, postSet1Checksum, "checksum should not include some_int_1 change")

	require.NoError(t, set.Set("some_duration_1", "10s"))
	postSet2Checksum := dflag.ChecksumFlagSet(set, filterOnlyDuration)
	t.Logf("post set2 checksum: %x", postSet2Checksum)
	assert.NotEqual(t, postSet1Checksum, postSet2Checksum, "checksum change when some_duration_1 changes")
}
