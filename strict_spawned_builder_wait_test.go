package evo_test

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// In Strict mode a container builder that hands a Wait to a goroutine it
// starts (the errgroup shape) gets the refusal back as the Wait's error. The
// misuse is recorded without a panic, because nothing recovers a panic on that
// goroutine and the process would end.
func TestStrictSpawnedBuilderWaitDoesNotEndTheProcess(t *testing.T) {
	if os.Getenv("EVO_STRICT_BUILDER_WAIT_CHILD") == "1" {
		out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Plain: true, Strict: true})
		other := out.Group("others").Task("other")
		other.Define(func(context.Context) error { return nil })
		built := out.Group("built")
		built.Define(func(g *evo.GroupHandle) {
			done := make(chan error, 1)
			go func() { done <- other.Wait() }()
			if err := <-done; err == nil || !strings.Contains(err.Error(), "inside a container builder") {
				os.Exit(3)
			}
			g.Task("child").Define(func(context.Context) error { return nil })
		})
		_ = built.Wait()
		os.Exit(0)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestStrictSpawnedBuilderWaitDoesNotEndTheProcess$")
	cmd.Env = append(os.Environ(), "EVO_STRICT_BUILDER_WAIT_CHILD=1")
	b, err := cmd.CombinedOutput()
	if err != nil {
		first, _, _ := strings.Cut(string(b), "\n")
		t.Fatalf("child process: %v; first line: %s", err, first)
	}
}
