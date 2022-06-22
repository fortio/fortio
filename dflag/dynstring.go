// Copyright 2015 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package dflag

import (
	"flag"
	"fmt"
	"regexp"
)

type DynStringValue = DynValue[string] // For backward compatibility

// DynString creates a `Flag` that represents `string` which is safe to change dynamically at runtime.
func DynString(flagSet *flag.FlagSet, name string, value string, usage string) *DynStringValue {
	return Dyn(flagSet, name, value, usage)
}

// ValidateDynStringMatchesRegex returns a validator function that checks all flag's values against regex.
func ValidateDynStringMatchesRegex(matcher *regexp.Regexp) func(string) error {
	return func(value string) error {
		if !matcher.MatchString(value) {
			return fmt.Errorf("value %v must match regex %v", value, matcher)
		}
		return nil
	}
}
