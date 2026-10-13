package process

import "testing"

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
