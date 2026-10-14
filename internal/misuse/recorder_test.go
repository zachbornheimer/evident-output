package misuse

import (
	"errors"
	"strings"
	"testing"
)

func TestFirstRecordedMisuseWins(t *testing.T) {
	r := New()
	r.Record(ErrClosed)
	r.RecordMisuseFor("task", ErrAlreadyResolved)
	if !errors.Is(r.First(), ErrClosed) {
		t.Errorf("First() = %v, want ErrClosed", r.First())
	}
}

func TestHintNamesARemedy(t *testing.T) {
	r := New()
	r.Note(ErrAlreadyResolved, "build", "failed")
	hint := r.Hint()
	for _, want := range []string{"resolve each task once", "build", "failed"} {
		if !strings.Contains(hint, want) {
			t.Errorf("Hint() = %q, want it to contain %q", hint, want)
		}
	}
}
