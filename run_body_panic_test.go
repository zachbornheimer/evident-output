package evo

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// bodyPanicChildEnv marks the child process that runs a panicking body.
const bodyPanicChildEnv = "EVO_TEST_BODY_PANIC_CHILD"

// A panic in the run body ends the process with that panic, but the run is
// first recorded as what happened: a panic, not a callback that exited
// through runtime.Goexit. The panic cannot be recovered around Execute (it
// comes from the goroutine Execute starts), so the body runs in a child.
func TestRunBodyPanicIsRecordedAsPanic(t *testing.T) {
	if os.Getenv(bodyPanicChildEnv) == "1" {
		Execute(context.Background(), Config{Name: "p", Output: Plain, Facades: Facades{Stderr: os.Stderr}},
			func(context.Context, *Run) error { panic("body boom") })
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestRunBodyPanicIsRecordedAsPanic$", "-test.count=1")
	cmd.Env = append(os.Environ(), bodyPanicChildEnv+"=1")
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Run(); err == nil {
		t.Fatalf("the child exited cleanly, want it to die with the body's panic:\n%s", output.String())
	}

	rendered := output.String()
	if strings.Contains(rendered, "Goexit") {
		t.Errorf("the run was recorded as a Goexit, not a panic:\n%s", rendered)
	}
	if !strings.Contains(rendered, "✗ p  panic: body boom") {
		t.Errorf("the run was not recorded as failing with the panic:\n%s", rendered)
	}
	if !strings.Contains(rendered, "goroutine ") {
		t.Errorf("the process did not die with the body's panic:\n%s", rendered)
	}
}
