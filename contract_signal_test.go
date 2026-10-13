//go:build unix

package evo_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

// The child half of TestSignalDuringDrainExits130 runs in a re-executed
// test binary, because a signal Main stopped listening for would kill the
// process that receives it.
const (
	signalChildEnv    = "EVO_CONTRACT_SIGNAL_CHILD"
	signalMarkerEnv   = "EVO_CONTRACT_SIGNAL_MARKER"
	signalTaskLength  = 10 * time.Second
	signalStartWithin = 10 * time.Second
	signalExitWithin  = 3 * time.Second
	signalPoll        = 10 * time.Millisecond
)

// signalChild is the program under test: Main with a body that declares one
// long Task and returns at once, so the signal arrives while Evo drains it.
func signalChild(marker string) int {
	return evo.Main(func(_ context.Context, r *evo.Run) error {
		r.Task("long").Define(func(ctx context.Context) error {
			if err := os.WriteFile(marker, nil, 0o600); err != nil {
				return err
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(signalTaskLength):
				return nil
			}
		})
		return nil
	})
}

// TestSignalDuringDrainExits130 pins CONTRACT.md §1: Main honours SIGINT and
// SIGTERM until the run has finished, including while declared work runs
// after the body returned, and exits 130.
func TestSignalDuringDrainExits130(t *testing.T) {
	if marker := os.Getenv(signalMarkerEnv); os.Getenv(signalChildEnv) != "" {
		os.Exit(signalChild(marker))
	}
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
		for _, mode := range []string{"human", "plain", "json", "jsonl"} {
			t.Run(sig.String()+"/"+mode, func(t *testing.T) {
				code, after := signalChildDuringDrain(t, sig, mode)
				if code != 130 {
					t.Errorf("exit code = %d, want 130", code)
				}
				if after > signalExitWithin {
					t.Errorf("exited %s after the signal, want within %s", after.Round(time.Millisecond), signalExitWithin)
				}
			})
		}
	}
}

// signalChildDuringDrain starts the child, signals it once its Task runs,
// and returns its exit code and how long after the signal it exited. A child
// still running signalExitWithin after the signal is killed.
func signalChildDuringDrain(t *testing.T, sig syscall.Signal, mode string) (int, time.Duration) {
	t.Helper()
	marker := filepath.Join(t.TempDir(), "started")
	cmd := exec.Command(os.Args[0], "-test.run=^TestSignalDuringDrainExits130$", "-test.count=1")
	cmd.Env = append(os.Environ(), signalChildEnv+"=1", signalMarkerEnv+"="+marker, "EVO_OUTPUT="+mode)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	deadline := time.Now().Add(signalStartWithin)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			<-done
			t.Fatalf("the child's Task never started")
		}
		time.Sleep(signalPoll)
	}

	if err := cmd.Process.Signal(sig); err != nil {
		t.Fatalf("signal child: %v", err)
	}
	signalled := time.Now()
	select {
	case err := <-done:
		return exitCode(t, err), time.Since(signalled)
	case <-time.After(signalExitWithin):
		_ = cmd.Process.Kill()
		err := <-done
		return exitCode(t, err), time.Since(signalled)
	}
}

func exitCode(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("wait child: %v", err)
	}
	return exitErr.ExitCode()
}
