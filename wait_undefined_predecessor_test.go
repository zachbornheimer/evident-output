package evo

import (
	"context"
	"io"
	"testing"
	"time"
)

// waitReturnDeadline bounds a Wait these tests expect to return promptly.
// A Wait that hangs is the defect, so the bound is generous, not tight.
const waitReturnDeadline = 5 * time.Second

// waitWithin runs wait on its own goroutine and fails the test when it
// has not returned by waitReturnDeadline.
func waitWithin(t *testing.T, what string, wait func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(waitReturnDeadline):
		t.Fatalf("%s: Wait did not return within %s", what, waitReturnDeadline)
		return nil
	}
}

func isolatedOutput(t *testing.T) *Run {
	t.Helper()
	out := initRun(Config{stateDir: t.TempDir(), stdout: io.Discard, stderr: io.Discard})
	t.Cleanup(func() { _ = out.close() })
	return out
}

func noop(context.Context) error { return nil }
