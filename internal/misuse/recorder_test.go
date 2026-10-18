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

func TestAllJoinsEveryDistinctMisuseInRecordOrder(t *testing.T) {
	r := New()
	if r.All() != nil {
		t.Fatalf("All() on an empty Recorder = %v, want nil", r.All())
	}
	r.Record(ErrClosed)
	r.RecordMisuseFor("a", ErrAlreadyResolved)
	r.RecordMisuseFor("b", ErrAlreadyResolved)
	r.Record(ErrInvalidProgress)
	all := r.All()
	if !errors.Is(all, ErrClosed) || !errors.Is(all, ErrAlreadyResolved) || !errors.Is(all, ErrInvalidProgress) {
		t.Fatalf("All() = %v, want it to wrap all three recorded sentinels", all)
	}
	want := strings.Join([]string{ErrClosed.Error(), ErrAlreadyResolved.Error(), ErrInvalidProgress.Error()}, "\n")
	if all.Error() != want {
		t.Errorf("All() = %q, want %q: record order, a repeated misuse kept once", all.Error(), want)
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
