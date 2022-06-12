// Copyright (c) Fortio Authors, All Rights Reserved
// See LICENSE for licensing terms. (Apache-2.0)

package dflag

import (
	"flag"
	"testing"
)

// Additional generic tests, most tests are covered by the old per type tests.

func TestParse_BadType(t *testing.T) {
	_, err := Parse[uint8]("23")
	assert.Error(t, err, "Expecting unpected type error")
	assert.Equal(t, err.Error(), "unexpected type uint8", "message/error should match")
}

func TestDflag_NonDynamic(t *testing.T) {
	set := flag.NewFlagSet("foobar", flag.ContinueOnError)
	set.Bool("notdyn", false, "...")
	static := set.Lookup("notdyn")
	assert.True(t, static != nil)
	assert.False(t, IsFlagDynamic(static))
}
