package manifest

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// lockHolderEnv names the state directory a re-executed test binary holds
// the manifest lock on (TestHelperProcessHoldsManifestLock).
const lockHolderEnv = "EVO_MANIFEST_TEST_LOCK_HOLDER"

// lockHolderReady is the line the holder prints once it holds the lock.
const lockHolderReady = "held"

// testLockWait is short so the busy path costs the suite little time.
const testLockWait = 250 * time.Millisecond

// openGuard is how long a test waits on Open before calling it hung.
const openGuard = 10 * time.Second

// TestHelperProcessHoldsManifestLock is not a test: re-executed with
// lockHolderEnv set, it holds the manifest lock until its stdin closes.
func TestHelperProcessHoldsManifestLock(t *testing.T) {
	dir := os.Getenv(lockHolderEnv)
	if dir == "" {
		t.Skip("lock holder helper; runs only when re-executed")
	}
	s, err := Open(context.Background(), Config{StateDir: dir}, fakeEnvironment{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(os.Stdout, lockHolderReady+"\n"); err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

// holdLockInAnotherProcess re-executes this test binary to hold dir's
// manifest lock and returns once the child reports it holds it. The child
// releases the lock when the test ends.
func holdLockInAnotherProcess(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperProcessHoldsManifestLock$")
	cmd.Env = append(os.Environ(), lockHolderEnv+"="+dir)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = stdin.Close()
		_ = cmd.Wait()
	})
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != lockHolderReady {
		t.Fatalf("lock holder did not report the lock held: %q, %v", line, err)
	}
}

type openResult struct {
	store *Store
	err   error
}

// openWithin runs Open on its own goroutine and fails t if it has not
// returned within openGuard, so a regression to an unbounded wait fails
// the test instead of hanging the suite.
func openWithin(t *testing.T, ctx context.Context, cfg Config) (*Store, time.Duration, error) {
	t.Helper()
	start := time.Now()
	done := make(chan openResult, 1)
	go func() {
		s, err := Open(ctx, cfg, fakeEnvironment{})
		done <- openResult{s, err}
	}()
	select {
	case r := <-done:
		return r.store, time.Since(start), r.err
	case <-time.After(openGuard):
		t.Fatalf("Open still blocked on a held manifest lock after %s", openGuard)
		return nil, 0, nil
	}
}

// TestOpenDegradesWhenAnotherProcessHoldsTheLock proves a lock held by
// another process costs Open at most LockWait: it then returns a Store
// with no history whose Warning names ErrBusy, and that Store completes a
// commit without writing the manifest the other run owns.
func TestOpenDegradesWhenAnotherProcessHoldsTheLock(t *testing.T) {
	dir := t.TempDir()
	holdLockInAnotherProcess(t, dir)

	s, elapsed, err := openWithin(t, context.Background(), Config{StateDir: dir, LockWait: testLockWait})
	if err != nil {
		t.Fatalf("Open = %v, want a Store that runs without history", err)
	}
	if elapsed < testLockWait {
		t.Fatalf("Open returned after %s, before the %s bound", elapsed, testLockWait)
	}
	if !errors.Is(s.Warning(), ErrBusy) {
		t.Fatalf("Warning = %v, want ErrBusy", s.Warning())
	}
	task := TaskRecord{Key: "t1", Operations: []OperationRecord{{Kind: "file", DefinitionFingerprint: "abc"}}}
	if err := s.CommitTask(context.Background(), ApplicationRecord{ID: "app"}, task); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, manifestFileName)); !os.IsNotExist(err) {
		t.Fatalf("a Store without the lock wrote the manifest (stat err = %v)", err)
	}
}

// TestOpenReturnsPromptlyWhenCancelledWhileWaiting proves cancellation
// ends the wait at once, long before LockWait, as an error.
func TestOpenReturnsPromptlyWhenCancelledWhileWaiting(t *testing.T) {
	dir := t.TempDir()
	holder, err := Open(context.Background(), Config{StateDir: dir}, fakeEnvironment{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Close() }()

	const cancelAfter = 50 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), cancelAfter)
	defer cancel()
	_, elapsed, err := openWithin(t, ctx, Config{StateDir: dir, LockWait: openGuard})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Open = %v, want context.DeadlineExceeded", err)
	}
	if elapsed > time.Second {
		t.Fatalf("Open took %s to notice cancellation", elapsed)
	}
}
