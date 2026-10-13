package misuse

import (
	"errors"
	"strings"
	"testing"
)

func TestFirstRecordedMisuseWins(t *testing.T) {
	r := New(false)
	r.Record(ErrClosed)
	r.RecordMisuseFor("task", ErrAlreadyResolved)
	if !errors.Is(r.First(), ErrClosed) {
		t.Errorf("First() = %v, want ErrClosed", r.First())
	}
}

func TestStrictNotePanicsAfterRecording(t *testing.T) {
	r := New(true)
	defer func() {
		got := recover()
		if !errors.Is(errorOf(got), ErrClosed) {
			t.Errorf("panic = %v, want ErrClosed", got)
		}
		if !errors.Is(r.First(), ErrClosed) {
			t.Errorf("First() after panic = %v, want ErrClosed", r.First())
		}
	}()
	r.Record(ErrClosed)
	t.Fatal("strict Record did not panic")
}

func TestKeepDoesNotPanicWhenStrict(t *testing.T) {
	r := New(true)
	r.Keep(ErrWaitDeadlock, "waiter", "")
	if !errors.Is(r.First(), ErrWaitDeadlock) {
		t.Errorf("First() = %v, want ErrWaitDeadlock", r.First())
	}
}

func TestHintNamesARemedy(t *testing.T) {
	r := New(false)
	r.Note(ErrAlreadyResolved, "build", "failed")
	hint := r.Hint()
	for _, want := range []string{"resolve each task once", "build", "failed"} {
		if !strings.Contains(hint, want) {
			t.Errorf("Hint() = %q, want it to contain %q", hint, want)
		}
	}
}

func errorOf(v any) error {
	err, _ := v.(error)
	return err
}
