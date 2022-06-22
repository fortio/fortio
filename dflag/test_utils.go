// Copyright 2022 Fortio Authors

// Only in "dflag" for sharing with tests.
// As tests don't fail, coverage of this particular file is poor.

package dflag

import (
	"fmt"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// --- Start of replacement for "github.com/stretchr/testify/assert"

// require.* and suite.* are used in _test packages while assert is used in dflag itself.
var assert = Testify{}

// ObjectsAreEqualValues returns true if a == b (through refection).
func (d *Testify) ObjectsAreEqualValues(a, b interface{}) bool {
	return reflect.DeepEqual(a, b)
}

// Testify is a short replacement for github.com/stretchr/testify/assert.
type Testify struct{}

// Errorf is a local variant to get the right line numbers.
func Errorf(t *testing.T, format string, rest ...interface{}) {
	_, file, line, _ := runtime.Caller(2)
	file = file[strings.LastIndex(file, "/")+1:]
	fmt.Printf("%s:%d %s", file, line, fmt.Sprintf(format, rest...))
	t.Fail()
}

// NotEqual checks for a not equal b.
func (d *Testify) NotEqual(t *testing.T, a, b interface{}, msg ...string) {
	if d.ObjectsAreEqualValues(a, b) {
		Errorf(t, "%v unexpectedly equal: %v", a, msg)
	}
}

// EqualValues checks for a equal b.
func (d *Testify) EqualValues(t *testing.T, a, b interface{}, msg ...string) {
	if !d.ObjectsAreEqualValues(a, b) {
		Errorf(t, "%v unexpectedly not equal %v: %v", a, b, msg)
	}
}

// Equal also checks for a equal b.
func (d *Testify) Equal(t *testing.T, a, b interface{}, msg ...string) {
	d.EqualValues(t, a, b, msg...)
}

// NoError checks for no errors (nil).
func (d *Testify) NoError(t *testing.T, err error, msg ...string) {
	if err != nil {
		Errorf(t, "expecting no error, got %v: %v", err, msg)
	}
}

// Error checks/expects an error.
func (d *Testify) Error(t *testing.T, err error, msg ...string) {
	if err == nil {
		Errorf(t, "expecting an error, didn't get it: %v", msg)
	}
}

// True checks bool is true.
func (d *Testify) True(t *testing.T, b bool, msg ...string) {
	if !b {
		Errorf(t, "expecting true, didn't: %v", msg)
	}
}

// False checks bool is false.
func (d *Testify) False(t *testing.T, b bool, msg ...string) {
	if b {
		Errorf(t, "expecting false, didn't: %v", msg)
	}
}

// Contains checks that needle is in haystack.
func (d *Testify) Contains(t *testing.T, haystack, needle string, msg ...string) {
	if !strings.Contains(haystack, needle) {
		Errorf(t, "%v doesn't contain %v: %v", haystack, needle, msg)
	}
}

// Fail fails the test.
func (d *Testify) Fail(t *testing.T, msg string) {
	t.Fatal(msg)
}

type hasT interface {
	T() *testing.T
	SetT(*testing.T)
}

// TestSuite to be used as base struct for test suites.
type TestSuite struct {
	t *testing.T
}

// T returns the current testing.T.
func (s *TestSuite) T() *testing.T {
	return s.t
}

// SetT sets the testing.T in the suite object.
func (s *TestSuite) SetT(t *testing.T) {
	s.t = t
}

type hasSetupTest interface {
	SetupTest()
}
type hasTearDown interface {
	TearDownTest()
}

// Run runs the test suite with SetupTest first and TearDownTest after.
func (d *Testify) Run(t *testing.T, suite hasT) {
	suite.SetT(t)
	tests := []testing.InternalTest{}
	methodFinder := reflect.TypeOf(suite)
	var setup hasSetupTest
	if s, ok := suite.(hasSetupTest); ok {
		setup = s
	}
	var tearDown hasTearDown
	if td, ok := suite.(hasTearDown); ok {
		tearDown = td
	}
	for i := 0; i < methodFinder.NumMethod(); i++ {
		method := methodFinder.Method(i)
		if ok, _ := regexp.MatchString("^Test", method.Name); !ok {
			continue
		}
		test := testing.InternalTest{
			Name: method.Name,
			F: func(t *testing.T) {
				method.Func.Call([]reflect.Value{reflect.ValueOf(suite)})
			},
		}
		tests = append(tests, test)
	}
	for _, test := range tests {
		if setup != nil {
			setup.SetupTest()
		}
		t.Run(test.Name, test.F)
		if tearDown != nil {
			tearDown.TearDownTest()
		}
	}
}

// --- End of replacement for "github.com/stretchr/testify/assert"
