// Copyright 2015 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package dflag

import (
	"flag"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFileFlag_ReadsWithADefault(t *testing.T) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	dynFlag, _ := DynJSON(set, "some_json_1", defaultJSONOne, "Use it or lose it").WithFileFlag("testdata/fileread_good.json")
	assert.EqualValues(t, defaultJSONOne, dynFlag.Get(), "value must be default after create, but before reading files")
	assert.NoError(t, ReadFileFlags(set), "reading from a file should succeed")
	assert.EqualValues(t,
		&outerJSON{FieldInts: []int{42}, FieldString: "new-value", FieldInner: &innerJSON{FieldBool: false}},
		dynFlag.Get(),
		"value must be set after reading from file")
}

func TestFileFlag_EmptyPathsAreIgnored(t *testing.T) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	dynFlag, _ := DynJSON(set, "some_json_1", defaultJSONOne, "Use it or lose it").WithFileFlag("")
	assert.EqualValues(t, defaultJSONOne, dynFlag.Get(), "value must be default after create, but before reading files")
	assert.NoError(t, ReadFileFlags(set), "reading from a file should succeed")
	assert.EqualValues(t, defaultJSONOne, dynFlag.Get(), "value must be default after read from file")
}

func TestFileFlag_BadFlagPathThenGood(t *testing.T) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	dynFlag, fileFlag := DynJSON(set, "some_json_1", defaultJSONOne, "Use it or lose it").WithFileFlag("testdata/unknown.json")
	assert.Error(t, ReadFileFlags(set), "reading from must not succeed for an unknown json")
	assert.EqualValues(t, defaultJSONOne, dynFlag.Get(), "value must be default after failed to read a file")
	// try changing the path to good one
	fileFlag.Set("testdata/fileread_good.json")
	assert.NoError(t, ReadFileFlags(set), "reading from a corrected set file should succeed")
}

func TestFileFlag_BadFileContent(t *testing.T) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	dynFlag, _ := DynJSON(set, "some_json_1", defaultJSONOne, "Use it or lose it").WithFileFlag("testdata/fileread_bad.json")
	assert.Error(t, ReadFileFlags(set), "reading from must not succeed for an unknown json")
	assert.EqualValues(t, defaultJSONOne, dynFlag.Get(), "value must be default after failed to read a file")
}
