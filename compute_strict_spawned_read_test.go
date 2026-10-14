package evo

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// Strict mode, an unordered callback whose own child goroutine reads a
// Computed. The read is refused, and the goroutine that made it has no recover
// to take a strict panic, so the Task fails with ErrComputedUnordered instead
// of the whole process ending.
func TestStrictSpawnedUnorderedReadFailsTheTaskInsteadOfEndingTheProcess(t *testing.T) {
	if os.Getenv("EVO_STRICT_SPAWNED_CHILD") == "1" {
		out := initRun(Config{stdout: io.Discard, plain: true, strict: true})
		made := out.Group("producers").Task("make")
		producer := Compute(made, func(context.Context) (int, error) { return 1, nil })
		_ = made.wait()
		read := out.Group("consumers").Task("read")
		read.Define(func(context.Context) error {
			done := make(chan struct{})
			go func() { defer close(done); _ = producer.Get() }()
			<-done
			return nil
		})
		err := read.wait()
		if errors.Is(err, ErrComputedUnordered) {
			os.Exit(0)
		}
		os.Exit(3)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestStrictSpawnedUnorderedReadFailsTheTaskInsteadOfEndingTheProcess$")
	cmd.Env = append(os.Environ(), "EVO_STRICT_SPAWNED_CHILD=1")
	b, err := cmd.CombinedOutput()
	if err != nil {
		first, _, _ := strings.Cut(string(b), "\n")
		t.Fatalf("child process: %v; first line: %s", err, first)
	}
}
