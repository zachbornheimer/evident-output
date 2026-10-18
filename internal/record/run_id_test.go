package record

import (
	"testing"
	"time"
)

func TestNewRunIDDiffersByTimeProcessAndSequence(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	base := NewRunID(at, 100, 1)
	for name, other := range map[string]string{
		"time":     NewRunID(at.Add(time.Nanosecond), 100, 1),
		"process":  NewRunID(at, 101, 1),
		"sequence": NewRunID(at, 100, 2),
	} {
		if other == base {
			t.Errorf("run id does not change with %s: %q", name, base)
		}
	}
	if again := NewRunID(at, 100, 1); again != base {
		t.Errorf("run id is not a pure function of its inputs: %q then %q", base, again)
	}
}
