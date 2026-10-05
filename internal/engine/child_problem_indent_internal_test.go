package engine

import (
	"context"
	"strings"
	"testing"
)

// TestChildProblemsSitUnderTheChildRow: a Sequence child's Problem tree is
// evidence under that child, one level in from the child's glyph, exactly
// as a root row's Problems sit under the root glyph. At the child's own
// column it read as a sibling row.
func TestChildProblemsSitUnderTheChildRow(t *testing.T) {
	got := finishedPlain(t, func(o *Output) {
		o.Task("root").Define(noop)
		seq := o.Sequence("seq")
		seq.Task("one").Define(noop)
		two := seq.Task("two")
		two.Define(func(context.Context) error {
			two.Fail("bad thing", Detail("line1\nline2"))
			return nil
		})
		seq.Task("three").Define(noop)
	})
	want := strings.Join([]string{
		"   ✗ two    bad thing",
		"      └─ line1",
		"         line2",
		"   - three  not started",
	}, "\n")
	if !strings.Contains(got, want) {
		t.Fatalf("want the child's Problems nested under it:\n%s\ngot:\n%s", want, got)
	}
}
