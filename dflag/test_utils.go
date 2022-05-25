// Copyright 2022 Fortio Authors

// Only in "dflag" for sharing with tests.
// As tests don't fail, coverage of this particular file is poor.

package dflag

import (
	"fmt"
	"reflect"
	"regexp"
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

// NotEqual checks for a not equal b.
func (d *Testify) NotEqual(t *testing.T, a, b interface{}, msg ...string) {
	if d.ObjectsAreEqualValues(a, b) {
		t.Errorf("%v unexpectedly equal: %v", a, msg)
	}
}

// EqualValues checks for a equal b.
func (d *Testify) EqualValues(t *testing.T, a, b interface{}, msg ...string) {
	if !d.ObjectsAreEqualValues(a, b) {
		t.Errorf("%v unexpectedly not equal %v: %v", a, b, msg)
	}
}

// Equal also checks for a equal b.
func (d *Testify) Equal(t *testing.T, a, b interface{}, msg ...string) {
	d.EqualValues(t, a, b, msg...)
}

// NoError checks for no errors (nil).
func (d *Testify) NoError(t *testing.T, err error, msg ...string) {
	if err != nil {
		t.Errorf("expecting no error, got %v: %v", err, msg)
	}
}

// Error checks/expects an error.
func (d *Testify) Error(t *testing.T, err error, msg ...string) {
	if err == nil {
		t.Errorf("expecting and error, didn't get it: %v", msg)
	}
}

// True checks bool is true.
func (d *Testify) True(t *testing.T, b bool, msg ...string) {
	if !b {
		t.Errorf("expecting true, didn't: %v", msg)
	}
}

// False checks bool is false.
func (d *Testify) False(t *testing.T, b bool, msg ...string) {
	if b {
		t.Errorf("expecting false, didn't: %v", msg)
	}
}

// Contains checks that needle is in haystack.
func (d *Testify) Contains(t *testing.T, haystack, needle string, msg ...string) {
	if !strings.Contains(haystack, needle) {
		t.Errorf("%v doesn't contain %v: %v", haystack, needle, msg)
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

type TestSuite struct {
	t *testing.T
}

func (s *TestSuite) T() *testing.T {
	return s.t
}

func (s *TestSuite) SetT(t *testing.T) {
	s.t = t
}

type hasSetupTest interface {
	SetupTest()
}
type hasTearDown interface {
	TearDownTest()
}

func (d *Testify) Run(t *testing.T, suite hasT) {
	suite.SetT(t)
	tests := []testing.InternalTest{}
	methodFinder := reflect.TypeOf(suite)
	var tearDown hasTearDown
	for i := 0; i < methodFinder.NumMethod(); i++ {
		method := methodFinder.Method(i)
		if ok, _ := regexp.MatchString("^Test", method.Name); !ok {
			continue
		}
		if setup, ok := suite.(hasSetupTest); ok {
			setup.SetupTest()
			continue
		}
		if td, ok := suite.(hasTearDown); ok {
			tearDown = td
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
		fmt.Printf("calling %s\n", test.Name)
		t.Run(test.Name, test.F)
	}
	if tearDown != nil {
		tearDown.TearDownTest()
	}
}

// --- End of replacement for "github.com/stretchr/testify/assert"
