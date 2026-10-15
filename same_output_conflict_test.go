package evo

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/internal/change"
)

// TestSameOutputConflictFailsBothTasks pins that two Tasks writing one File
// path are a program error reported on both: each Task fails with
// ErrFileConflictingProducer, naming the other, and the run fails, whichever
// Task claimed the path first.
func TestSameOutputConflictFailsBothTasks(t *testing.T) {
	out := newGraphTestOutput(t)
	assertSameOutputConflictFailsBoth(t, out, out.Group("writers").Task)
}

// TestSameOutputConflictFailsEarlierSettledTask pins the same rule when the
// producers cannot overlap: a Sequence settles the first Task Done before the
// second claims the path, so the first Task fails after it settled.
func TestSameOutputConflictFailsEarlierSettledTask(t *testing.T) {
	out := newGraphTestOutput(t)
	assertSameOutputConflictFailsBoth(t, out, out.Sequence("writers").Task)
}

// assertSameOutputConflictFailsBoth declares Tasks "a" and "b" through
// declare, both writing one File path, and checks the rule on both.
func assertSameOutputConflictFailsBoth(t *testing.T, out *Run, declare func(string) *Task) {
	t.Helper()
	shared := filepath.Join(t.TempDir(), "shared.txt")
	tasks := map[string]*Task{}
	for _, name := range []string{"a", "b"} {
		tasks[name] = declare(name)
		tasks[name].Define(func(ctx context.Context) error {
			return File(ctx, FileSpec{Path: shared, Contents: []byte(name)})
		})
	}
	err := closeWithin(t, out)
	if err == nil {
		t.Fatal("Finish = nil, want the run to fail on the conflicting outputs")
	}
	for name, other := range map[string]string{"a": "b", "b": "a"} {
		st := tasks[name].snapshot()
		if st.State != failed {
			t.Errorf("Task %s state = %v, want failed", name, st.State)
		}
		if !strings.Contains(st.Summary, other) || !strings.Contains(st.Summary, "claimed by") {
			t.Errorf("Task %s failure %q does not name Task %s", name, st.Summary, other)
		}
	}
	if !errors.Is(err, change.ErrFileConflictingProducer) || !strings.Contains(err.Error(), `"a"`) || !strings.Contains(err.Error(), `"b"`) {
		t.Errorf("Finish error = %v, want ErrFileConflictingProducer naming both Tasks", err)
	}
	if got := out.conclusion().exitCode; got != exitFailed {
		t.Errorf("exit code = %d, want %d", got, exitFailed)
	}
}

// TestExecOutputConflictNamesExec pins that a same-output conflict names the
// operation each Task used to claim the path, not always File: an Exec over a
// File's path, a File over an Exec's path and two Execs each read as what they are.
func TestExecOutputConflictNamesExec(t *testing.T) {
	for _, tc := range []struct {
		name          string
		first, second string
		want          []string
	}{
		{"exec over file", "File", "Exec", []string{"this Exec", "File in Task"}},
		{"file over exec", "Exec", "File", []string{"this File", "Exec in Task"}},
		{"exec over exec", "Exec", "Exec", []string{"this Exec", "Exec in Task"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			tool := execFixture(t, dir, "tool", "v1")
			shared := filepath.Join(dir, "shared.txt")
			out := initRun(Config{State: State{Dir: t.TempDir()}, Facades: Facades{Process: &scriptedRunner{exitCode: 0}}})
			t.Cleanup(func() { _ = out.close() })
			claim := func(op string) func(context.Context) error {
				return func(ctx context.Context) error {
					if op == "File" {
						return File(ctx, FileSpec{Path: shared, Contents: []byte("x")})
					}
					_, err := Exec(ctx, ExecSpec{executable: tool, Dir: dir, Outputs: []string{"shared.txt"}})
					return err
				}
			}
			first := out.Task("first")
			first.Define(claim(tc.first))
			_ = first.wait()
			var secondErr error
			second := out.Task("second")
			second.Define(func(ctx context.Context) error {
				secondErr = claim(tc.second)(ctx)
				return secondErr
			})
			_ = second.wait()

			if !errors.Is(secondErr, change.ErrFileConflictingProducer) {
				t.Fatalf("second claim err = %v, want ErrFileConflictingProducer", secondErr)
			}
			for _, want := range tc.want {
				if !strings.Contains(secondErr.Error(), want) {
					t.Errorf("conflict %q does not contain %q", secondErr, want)
				}
			}
			if strings.Contains(secondErr.Error(), "File output path") {
				t.Errorf("conflict %q still calls every output a File output", secondErr)
			}
		})
	}
}
