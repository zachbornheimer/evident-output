package engine

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// finishedPlain runs declare on a plain Output and returns what Finish
// printed.
func finishedPlain(t *testing.T, declare func(o *Output)) string {
	t.Helper()
	var buf bytes.Buffer
	out := Init(Config{Isolated: true, Plain: true, Color: ColorNever, Stdout: &buf, Stderr: &buf})
	t.Cleanup(func() { _ = out.Close() })
	declare(out)
	_ = out.Finish()
	return buf.String()
}

// TestDuplicateSiblingRowStatesItOnce: the refusal row is named for the
// duplicated name and says "duplicate task name" once; the misuse line
// names the duplicate.
func TestDuplicateSiblingRowStatesItOnce(t *testing.T) {
	got := finishedPlain(t, func(o *Output) {
		o.Task("t").Define(noop)
		o.Task("t")
	})
	if !strings.Contains(got, "t  duplicate task name\n") {
		t.Errorf("want the row `t  duplicate task name`:\n%s", got)
	}
	if n := strings.Count(got, "duplicate task name"); n != 1 {
		t.Errorf("\"duplicate task name\" appears %d times, want 1:\n%s", n, got)
	}
	if !strings.Contains(got, "duplicate sibling name: t") {
		t.Errorf("misuse line does not name the duplicate:\n%s", got)
	}
}

// TestDuplicateSiblingNestedRowStatesItOnce is the nested form.
func TestDuplicateSiblingNestedRowStatesItOnce(t *testing.T) {
	got := finishedPlain(t, func(o *Output) {
		p := o.Group("p")
		p.Sequence("x").Task("a").Define(noop)
		p.Sequence("x")
	})
	if n := strings.Count(got, "duplicate sequence name"); n != 1 {
		t.Errorf("\"duplicate sequence name\" appears %d times, want 1:\n%s", n, got)
	}
}

// TestRootContainerNamesShareOneRegistry: a Group and a Sequence with one
// name under the same parent are duplicate siblings at the root exactly as
// they are nested (§3.1).
func TestRootContainerNamesShareOneRegistry(t *testing.T) {
	for _, level := range []string{"root", "nested"} {
		t.Run(level, func(t *testing.T) {
			out := quietOutput(Config{})
			t.Cleanup(func() { _ = out.Close() })
			var seq *SequenceHandle
			if level == "root" {
				out.Group("x")
				seq = out.Sequence("x")
			} else {
				p := out.Group("p")
				p.Group("x")
				seq = p.Sequence("x")
			}
			if !errors.Is(seq.tasks.rejected, ErrDuplicateSiblingName) {
				t.Fatalf("Sequence(\"x\") after Group(\"x\") rejected = %v, want ErrDuplicateSiblingName", seq.tasks.rejected)
			}
		})
	}
}
