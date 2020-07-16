// Copyright 2015 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package flagz

import (
	"testing"

	"flag"

	"github.com/stretchr/testify/assert"
)

func TestFileFlag_ReadsWithADefault(t *testing.T) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	dynFlag := DynJSON(set, "some_json_1", defaultJSON, "Use it or lose it").WithFileFlag("testdata/fileread_good.json")
	assert.EqualValues(t, defaultJSON, dynFlag.Get(), "value must be default after create, but before reading files")
	assert.NoError(t, ReadFileFlags(set), "reading from a file should succeed")
	assert.EqualValues(t,
		&outerJSON{FieldInts: []int{42}, FieldString: "new-value", FieldInner: &innerJSON{FieldBool: false}},
		dynFlag.Get(),
		"value must be set after reading from file")
}

func TestFileFlag_EmptyPathsAreIgnored(t *testing.T) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	dynFlag := DynJSON(set, "some_json_1", defaultJSON, "Use it or lose it").WithFileFlag("")
	assert.EqualValues(t, defaultJSON, dynFlag.Get(), "value must be default after create, but before reading files")
	assert.NoError(t, ReadFileFlags(set), "reading from a file should succeed")
	assert.EqualValues(t, defaultJSON, dynFlag.Get(), "value must be default after read from file")
}

func TestFileFlag_BadFlagPath(t *testing.T) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	dynFlag := DynJSON(set, "some_json_1", defaultJSON, "Use it or lose it").WithFileFlag("testdata/unknown.json")
	assert.Error(t, ReadFileFlags(set), "reading from must not succeed for an unknown json")
	assert.EqualValues(t, defaultJSON, dynFlag.Get(), "value must be default after failed to read a file")
}

func TestFileFlag_BadFileContent(t *testing.T) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	dynFlag := DynJSON(set, "some_json_1", defaultJSON, "Use it or lose it").WithFileFlag("testdata/fileread_bad.json")
	assert.Error(t, ReadFileFlags(set), "reading from must not succeed for an unknown json")
	assert.EqualValues(t, defaultJSON, dynFlag.Get(), "value must be default after failed to read a file")
}
