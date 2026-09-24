package engine

import (
	"context"
	"io"
	"path/filepath"
	"testing"
	"time"
)

// TestConcurrentCloseTearsDownOnce: a signal handler and a deferred Close
// can race. Only the first may release the manifest lock; the second must
// wait for that teardown and report nothing, not a double-release error.
func TestConcurrentCloseTearsDownOnce(t *testing.T) {
	dir := t.TempDir()
	out := Init(Config{Isolated: true, StateDir: dir, Stdout: io.Discard, Stderr: io.Discard})

	release := make(chan struct{})
	out.Task("write").Define(func(ctx context.Context) error {
		if err := File(ctx, FileSpec{Path: filepath.Join(dir, "a.txt"), Contents: []byte("a")}); err != nil {
			return err
		}
		<-release
		return nil
	})

	errs := make(chan error, 2)
	for range 2 {
		go func() { errs <- out.Close() }()
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	for range 2 {
		select {
		case err := <-errs:
			if err != nil {
				t.Errorf("Close() = %v, want nil from both concurrent calls", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Close hung")
		}
	}
}
