package evo_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func TestBehavioralAcceptance_DefineGroupFile_WritesFiles(t *testing.T) {
	dir := t.TempDir()
	left := filepath.Join(dir, "left.json")
	right := filepath.Join(dir, "right.json")
	out := isolatedOutput(t, nil)

	result := out.Run(context.Background(), func(ctx context.Context) error {
		files := out.Group("write configs")
		leftTask := files.Task("write left config")
		leftTask.Define(func(ctx context.Context) error {
			return evo.File(ctx, evo.FileSpec{Path: left, Contents: []byte("left\n")})
		})
		rightTask := files.Task("write right config")
		rightTask.Define(func(ctx context.Context) error {
			return evo.File(ctx, evo.FileSpec{Path: right, Contents: []byte("right\n")})
		})
		return nil
	})
	if result.Err != nil {
		t.Fatalf("Run: %v", result.Err)
	}
	if result.ExitCode() != 0 {
		t.Fatalf("exit=%d conclusion=%+v", result.ExitCode(), result.Conclusion)
	}
	if result.Conclusion.State != evo.StateChanged {
		t.Fatalf("state=%v, want StateChanged", result.Conclusion.State)
	}
	gotLeft, err := os.ReadFile(left)
	if err != nil || string(gotLeft) != "left\n" {
		t.Fatalf("left=%q err=%v", gotLeft, err)
	}
	gotRight, err := os.ReadFile(right)
	if err != nil || string(gotRight) != "right\n" {
		t.Fatalf("right=%q err=%v", gotRight, err)
	}
}

func TestBehavioralAcceptance_DefinePatchFile_CommitsDiff(t *testing.T) {
	dir := t.TempDir()
	path := writeGreeting(t, dir)
	out := isolatedOutput(t, nil)

	result := out.Run(context.Background(), func(ctx context.Context) error {
		task := out.Task("apply greeting")
		task.Define(func(ctx context.Context) error {
			patched, err := evo.Patch(ctx, evo.PatchSpec{Diff: greetingDiff(), Dir: dir})
			if err != nil {
				return err
			}
			for _, spec := range patched.Files {
				if err := evo.File(ctx, spec); err != nil {
					return err
				}
			}
			return nil
		})
		return nil
	})
	if result.Err != nil {
		t.Fatalf("Run: %v", result.Err)
	}
	if result.ExitCode() != 0 {
		t.Fatalf("exit=%d conclusion=%+v", result.ExitCode(), result.Conclusion)
	}
	if result.Conclusion.State != evo.StateChanged {
		t.Fatalf("state=%v, want StateChanged", result.Conclusion.State)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "hello\nthere\n" {
		t.Fatalf("got %q err=%v", got, err)
	}
	found := false
	for _, c := range result.Conclusion.Changes {
		for _, r := range c.Records {
			if r.Object == path {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("conclusion has no Effect for %q: %+v", path, result.Conclusion.Changes)
	}
}
