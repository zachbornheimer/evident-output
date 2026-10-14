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

// The child half of each signal test runs in a re-executed test binary,
// because a signal Main does not listen for would kill the process that
// receives it.
const (
	signalChildEnv    = "EVO_CONTRACT_SIGNAL_CHILD"
	signalMarkerEnv   = "EVO_CONTRACT_SIGNAL_MARKER"
	signalTaskLength  = 10 * time.Second
	signalStartWithin = 10 * time.Second
	signalExitWithin  = 3 * time.Second
	signalPoll        = 10 * time.Millisecond
)

// signalChild is a program under test: Main with a body that declares one
// long Task and returns at once, so the signal arrives while Evo drains it.
func signalChild(marker string, opts ...evo.DefineOption) int {
	return evo.Main(evo.Config{}, func(_ context.Context, r *evo.Run) error {
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
		}, opts...)
		return nil
	})
}

// runAsSignalChild runs program and exits when this process is the child a
// signal test started; otherwise it returns.
func runAsSignalChild(program func(marker string) int) {
	if os.Getenv(signalChildEnv) != "" {
		os.Exit(program(os.Getenv(signalMarkerEnv)))
	}
}

// TestSignalDuringDrainExits130 pins CONTRACT.md §1: Main honours SIGINT and
// SIGTERM until the run has finished, including while declared work runs
// after the body returned, and exits 130.
func TestSignalDuringDrainExits130(t *testing.T) {
	runAsSignalChild(func(marker string) int { return signalChild(marker) })
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
		for _, mode := range []string{"human", "plain", "json", "jsonl"} {
			t.Run(sig.String()+"/"+mode, func(t *testing.T) {
				assertSignalledChildExits(t, "TestSignalDuringDrainExits130", sig, mode, 130)
			})
		}
	}
}

// TestCleanStop_SignalEndsTheTaskSucceeded pins CONTRACT.md §2: a signal
// named in CleanStop ends that Task Succeeded, so the run exits 0, while a
// signal it does not name still cancels the run with exit 130.
func TestCleanStop_SignalEndsTheTaskSucceeded(t *testing.T) {
	runAsSignalChild(func(marker string) int { return signalChild(marker, evo.CleanStop(os.Interrupt)) })
	for _, tc := range []struct {
		sig  syscall.Signal
		want int
	}{{syscall.SIGINT, 0}, {syscall.SIGTERM, 130}} {
		for _, mode := range []string{"plain", "json"} {
			t.Run(tc.sig.String()+"/"+mode, func(t *testing.T) {
				assertSignalledChildExits(t, "TestCleanStop_SignalEndsTheTaskSucceeded", tc.sig, mode, tc.want)
			})
		}
	}
}

// assertSignalledChildExits re-runs testName as a child, signals it once its
// Task runs, and checks it exits want within signalExitWithin.
func assertSignalledChildExits(t *testing.T, testName string, sig syscall.Signal, mode string, want int) {
	t.Helper()
	code, killed := signalChildWhileRunning(t, testName, sig, mode)
	if killed {
		t.Errorf("the child was still running %s after %s; killed it", signalExitWithin, sig)
		return
	}
	if code != want {
		t.Errorf("exit code = %d after %s, want %d", code, sig, want)
	}
}

// signalChildWhileRunning starts testName as a child, signals it once its
// Task runs, and returns its exit code and whether it had to be killed.
func signalChildWhileRunning(t *testing.T, testName string, sig syscall.Signal, mode string) (int, bool) {
	t.Helper()
	marker := filepath.Join(t.TempDir(), "started")
	cmd := exec.Command(os.Args[0], "-test.run=^"+testName+"$", "-test.count=1")
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
	select {
	case err := <-done:
		return exitCode(t, err), false
	case <-time.After(signalExitWithin):
		_ = cmd.Process.Kill()
		err := <-done
		return exitCode(t, err), true
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
