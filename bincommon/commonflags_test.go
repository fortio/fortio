package bincommon

import "testing"

func TestConnectionReuseDynFlag(t *testing.T) {
	val := ConnectionReuseRange.Get()
	if val != "" {
		t.Errorf("Default value for connection reuse range should be empty.")
	}

	err := ConnectionReuseRange.Set("1:2:3:4")
	if err == nil {
		t.Errorf("Shoud fail when more than two values are provided for connection reuse range.")
	}

	err = ConnectionReuseRange.Set("foo")
	if err == nil {
		t.Errorf("Shoud fail when non integer value is provided for connection reuse range.")
	}

	err = ConnectionReuseRange.Set("")
	if err != nil {
		t.Errorf("Expect no error when no value is privided, got err: %v.", err)
	}

	err = ConnectionReuseRange.Set("10")
	if err != nil {
		t.Errorf("Expect no error when single value is privided, got err: %v.", err)
	}

	err = ConnectionReuseRange.Set("20:10")
	if err != nil {
		t.Errorf("Expect no error when two values are privided, got err: %v.", err)
	}

	if httpOpts.ConnReuseRange[0] > httpOpts.ConnReuseRange[1] {
		t.Errorf("Connection reuse min value should be smaller or equal to the max value.")
	}

	err = ConnectionReuseRange.Set("10:20")
	if err != nil {
		t.Errorf("Expect no error when two values are privided, got err: %v", err)
	}

	if httpOpts.ConnReuseRange[0] > httpOpts.ConnReuseRange[1] {
		t.Errorf("Connection reuse min value should be smaller or equal to the max value.")
	}
}
