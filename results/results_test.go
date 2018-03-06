package results

import (
	"time"
	"testing"
)

func TestID(t *testing.T) {
	var tests = []struct {
		labels string // input
		id     string // expected suffix after the date
	}{
		{"", ""},
		{"abcDEF123", "_abcDEF123"},
		{"A!@#$%^&*()-+=/'B", "_A_B"},
		// Ends with non alpha, skip last _
		{"A  ", "_A"},
		{" ", ""},
		// truncated to fit 64 (17 from date/time + _ + 46 from labels)
		{"123456789012345678901234567890123456789012345678901234567890", "_1234567890123456789012345678901234567890123456"},
	}
	startTime := time.Date(2001, time.January, 2, 3, 4, 5, 0, time.Local)
	prefix := "2001-01-02-030405"
	for _, tst := range tests {
		o := RunnerResults{
			StartTime: startTime,
			Labels:    tst.labels,
		}
		id := o.ID()
		expected := prefix + tst.id
		if id != expected {
			t.Errorf("id: got %s, not as expected %s", id, expected)
		}
	}
}
