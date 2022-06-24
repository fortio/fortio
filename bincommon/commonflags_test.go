package bincommon

import "testing"

func TestConnectionReuseDynFlag(t *testing.T) {
	val := ConnectionReuseRange.Get()
	if val != "" {
		t.Errorf("Default value for connection reuse range should be empty.")
	}

	value := "10:20"
	err := ConnectionReuseRange.Set(value)
	if err != nil {
		t.Errorf("Failed to set connection reuse range, err: %v", err)
	}

	if ConnectionReuseRange.Get() != value {
		t.Errorf("Connection reuse range doesn't match the set value, expected %v, got %v",
			ConnectionReuseRange.Get(), value)
	}

	value = "foo"
	err = ConnectionReuseRange.Set(value)
	if err == nil {
		t.Errorf("Should fail when invalid input was given")
	}
}
