// Copyright 2015 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package dflag

import (
	"flag"
	"fmt"
	"io/ioutil"
)

// ReadFileFlags parses the flagset to discover all "fileread" flags and evaluates them.
//
// By reading and evaluating it means: attempts to read the file and set the value.
func ReadFileFlags(flagSet *flag.FlagSet) error {
	var outerErr error
	flagSet.VisitAll(func(f *flag.Flag) {
		if frv, ok := f.Value.(*FileReadValue); ok {
			if err := frv.readFile(); err != nil {
				outerErr = fmt.Errorf("reading file flag '%v' failed: %w", f.Name, err)
			}
		}
	})
	return outerErr
}

// FileReadValue is a flag that wraps another flag and makes it readable from a local file in the filesystem.
type FileReadValue struct {
	DynamicFlagValueTag
	parentFlagName string
	filePath       string
	flagSet        *flag.FlagSet
}

// FileReadFlag creates a `Flag` that allows you to pass a flag.
//
// If defaultFilePath is non empty, the dflag.ReadFileFlags will expect the file to be there.
func FileReadFlag(flagSet *flag.FlagSet, parentFlagName string, defaultFilePath string) *FileReadValue {
	dynValue := &FileReadValue{
		DynamicFlagValueTag: DynamicFlagValueTag{},
		parentFlagName:      parentFlagName, filePath: defaultFilePath, flagSet: flagSet,
	}
	flagSet.Var(dynValue,
		parentFlagName+"_path",
		fmt.Sprintf("Path to read contents to a file to read contents of '%v' from.", parentFlagName))
	return dynValue
}

func (f *FileReadValue) String() string {
	return fmt.Sprintf("fileread_for(%v)", f.parentFlagName)
}

// Set updates the value from a string representation of the file path.
func (f *FileReadValue) Set(path string) error {
	f.filePath = path
	return nil
}

func (f *FileReadValue) readFile() error {
	if f.filePath == "" {
		return nil
	}
	data, err := ioutil.ReadFile(f.filePath)
	if err != nil {
		return err
	}
	return f.flagSet.Set(f.parentFlagName, string(data))
}
