package process

import (
	"slices"
	"testing"
	"time"
)

func TestRingRetainsCompletedLinesAndBoundsTheTail(t *testing.T) {
	t.Parallel()
	r := NewRing(2, 0)
	if _, err := r.Write([]byte("one\ntwo\nthree\n"), StreamCombined, nil); err != nil {
		t.Fatalf("Write: %v", err)
	}
	lines, truncated := r.Texts(StreamCombined, 0, nil)
	if !truncated {
		t.Fatal("want truncated after dropping the first line")
	}
	if got := JoinLines(lines, truncated); got != TruncationMarker+"\n"+"two\nthree" {
		t.Fatalf("Text = %q", got)
	}
}

func TestRingSeparateStreamsDoNotMergePendingFragments(t *testing.T) {
	t.Parallel()
	r := NewRing(0, 0)
	if _, err := r.Write([]byte("out"), StreamStdout, nil); err != nil {
		t.Fatalf("stdout: %v", err)
	}
	if _, err := r.Write([]byte("err"), StreamStderr, nil); err != nil {
		t.Fatalf("stderr: %v", err)
	}
	out, _ := r.Texts(StreamStdout, 0, nil)
	errLines, _ := r.Texts(StreamStderr, 0, nil)
	if len(out) != 1 || out[0] != "out" {
		t.Fatalf("stdout pending = %v", out)
	}
	if len(errLines) != 1 || errLines[0] != "err" {
		t.Fatalf("stderr pending = %v", errLines)
	}
	if !r.PreferStderr() {
		t.Fatal("want PreferStderr when both streams have content")
	}
}

func TestRingOnAppendSeesCompletedLinesOnly(t *testing.T) {
	t.Parallel()
	r := NewRing(0, 0)
	var seen []string
	r.SetOnAppend(func(line string) { seen = append(seen, line) })
	if _, err := r.Write([]byte("partial"), StreamCombined, nil); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if len(seen) != 0 {
		t.Fatalf("pending fragment notified %v", seen)
	}
	if err := r.Close([]Stream{StreamCombined}, nil); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if len(seen) != 1 || seen[0] != "partial" {
		t.Fatalf("completed = %v, want [partial]", seen)
	}
}

// onAppend runs with the ring's mutex released: it takes the run's lock,
// and a reader holding the run's lock reads the ring.
func TestRingCallsOnAppendOutsideItsLock(t *testing.T) {
	r := NewRing(0, 0)
	var seen []int
	r.SetOnAppend(func(string) { seen = append(seen, r.Len()) }) // re-enters r.mu
	done := make(chan struct{})
	go func() {
		_, _ = r.Write([]byte("one\ntwo\npartial"), StreamStdout, nil)
		_ = r.Close([]Stream{StreamStdout}, nil)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("onAppend ran with the ring's mutex held")
	}
	if want := []int{2, 2, 3}; !slices.Equal(seen, want) {
		t.Errorf("Len seen by onAppend = %v, want %v (every line kept before it is announced)", seen, want)
	}
}
