package evo

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// A Build callback that panics fails its container with the panic's text on
// the container's row, as a panicking Task callback does on its own.
func TestBuildPanicCarriesItsText(t *testing.T) {
	var stderr bytes.Buffer
	result := Execute(context.Background(), Config{Output: Plain, Facades: Facades{Stderr: &stderr}}, func(ctx context.Context, r *Run) error {
		g := r.Group("g")
		g.Build(func(*Group) { panic("builder boom") })
		r.Task("dep").After(g).Define(func(context.Context) error { return nil })
		return nil
	})
	rendered := stderr.String()
	if got := result.ExitCode(); got != int(exitFailed) {
		t.Errorf("exit code = %d, want ExitFailed (%d):\n%s", got, exitFailed, rendered)
	}
	if want := "✗ g  panic: builder boom"; !strings.Contains(rendered, want) {
		t.Errorf("output missing %q:\n%s", want, rendered)
	}
}
