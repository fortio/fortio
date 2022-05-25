// Copyright 2022 Fortio Authors

package dflag

import (
	"reflect"
	"strings"
	"testing"
)

// --- Start of replacement for "github.com/stretchr/testify/assert"

func (d *Testify) ObjectsAreEqualValues(a, b interface{}) bool {
	return reflect.DeepEqual(a, b)
}

type Testify struct{}

func (d *Testify) NotEqual(t *testing.T, a, b interface{}, msg ...string) {
	if d.ObjectsAreEqualValues(a, b) {
		t.Errorf("%v unexpectedly equal: %v", a, msg)
	}
}

func (d *Testify) EqualValues(t *testing.T, a, b interface{}, msg ...string) {
	if !d.ObjectsAreEqualValues(a, b) {
		t.Errorf("%v unexpectedly not equal %v: %v", a, b, msg)
	}
}

func (d *Testify) Equal(t *testing.T, a, b interface{}, msg ...string) {
	d.EqualValues(t, a, b, msg...)
}

func (d *Testify) NoError(t *testing.T, err error, msg ...string) {
	if err != nil {
		t.Errorf("expecting no error, got %v: %v", err, msg)
	}
}

func (d *Testify) Error(t *testing.T, err error, msg ...string) {
	if err == nil {
		t.Errorf("expecting and error, didn't get it: %v", msg)
	}
}

func (d *Testify) True(t *testing.T, b bool, msg ...string) {
	if !b {
		t.Errorf("expecting true, didn't: %v", msg)
	}
}

func (d *Testify) False(t *testing.T, b bool, msg ...string) {
	if b {
		t.Errorf("expecting false, didn't: %v", msg)
	}
}

func (d *Testify) Contains(t *testing.T, haystack, needle string, msg ...string) {
	if !strings.Contains(haystack, needle) {
		t.Errorf("%v doesn't contain %v: %v", haystack, needle, msg)
	}
}

func (d *Testify) Fail(t *testing.T, msg string) {
	t.Fatal(msg)
}

var (
	assert = Testify{}
)

// --- End of replacement for "github.com/stretchr/testify/assert"
