package evo

import (
	"bytes"
	"context"
	"testing"
)

// TestProblemWithSubjectPrintsHeadlineOnce pins that a Problem's text never
// prints twice: when the row already states it as its headline, the Problem's
// tree line carries the subject alone.
func TestProblemWithSubjectPrintsHeadlineOnce(t *testing.T) {
	cases := []struct {
		name  string
		build func(*Run)
		want  string
	}{
		{"advisory Problem on a Done Task", func(r *Run) {
			r.Task("audit").Problem("audit is advisory", On("audit.ok"), Code("advisory")).
				Define(func(context.Context) error { return nil })
		}, "! audit  audit is advisory\n   └─ audit.ok\n"},
		{"failure", func(r *Run) {
			r.Task("clean workspace").fail("permission denied", On("/var/cache"))
		}, "✗ clean workspace  permission denied\n   └─ /var/cache\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			out := initRun(Config{Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
			tc.build(out)
			_ = out.finish()
			if got := buf.String(); len(got) < len(tc.want) || got[:len(tc.want)] != tc.want {
				t.Fatalf("mismatch:\n--- want prefix ---\n%s\n--- got ---\n%s", tc.want, got)
			}
		})
	}
}
