package engine

import (
	"context"
	"testing"
)

// TestMain_ReturnsCodeWithoutExiting proves Main derives its exit code from
// Run without calling os.Exit itself (spec §1.1: "Main owns CLI
// signal-to-cancellation setup and returns the derived exit code; it does
// not itself call os.Exit") — the caller writes os.Exit(evo.Main(run)).
// 1.0 removed MainWith (the pre-v0.6 func(*Output) error entrypoint) and
// its exitProcess facade outright: nothing in the library calls os.Exit
// anymore, so there is no longer a facade to swap for a fake here.
func TestMain_ReturnsCodeWithoutExiting(t *testing.T) {
	SetDefault(Init(Config{Isolated: true}))
	code := Main(func(context.Context) error {
		Task("x").Done()
		return nil
	})
	if code != ExitOK {
		t.Fatalf("Main returned %d, want %d", code, ExitOK)
	}
}
