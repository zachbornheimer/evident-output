package evo

import (
	"context"
	"testing"
)

// TestMain_ReturnsCodeWithoutExiting proves Main derives its exit code from
// Run without calling os.Exit itself (spec §1.1: "Main owns CLI
// signal-to-cancellation setup and returns the derived exit code; it does
// not itself call os.Exit") — the caller writes os.Exit(evo.Main(run)).
// 1.0 removed MainWith (the pre-v0.6 func(*Run) error entrypoint) and
// its exitProcess facade outright: nothing in the library calls os.Exit
// anymore, so there is no longer a facade to swap for a fake here.
func TestMain_ReturnsCodeWithoutExiting(t *testing.T) {
	setDefault(initRun(Config{isolated: true}))
	code := Main(func(ctx context.Context, r *Run) error {
		defaultTask("x").succeed("")
		return nil
	})
	if code != exitOK {
		t.Fatalf("Main returned %d, want %d", code, exitOK)
	}
}
