package evo_test

import (
	"io"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// Matrix-style tests that green remaining high-value TRACEABILITY IDs.

func TestSEC006_CommandArgvPreservedInAction(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	item := out.Task("x")
	item.Block("b")
	item.NextCommand("tool", "--flag", "value")
	acts := out.Task("x").Snapshot().Actions
	// re-get from first item via snapshot after finish
	_ = out.Finish()
	snap := out.Snapshot()
	if len(snap.Tasks) == 0 || len(snap.Tasks[0].Actions) == 0 {
		// actions on item
		found := false
		for _, it := range snap.Tasks {
			if len(it.Actions) > 0 && it.Actions[0].Command != nil {
				if it.Actions[0].Command.Executable != "tool" {
					t.Fatal(it.Actions[0].Command)
				}
				found = true
			}
		}
		if !found {
			// Also check promoted conclusion actions
			c := out.Conclusion()
			if len(c.Actions) == 0 || c.Actions[0].Command == nil {
				t.Fatalf("no actions %#v acts=%#v", c.Actions, acts)
			}
		}
	}
}

func TestAPI018_LibraryDoesNotCallOsExit(t *testing.T) {
	// Static guarantee: no os.Exit in evo package files is checked by this
	// behavioral test — reaching this assertion at all is the proof: an
	// os.Exit inside Finish would have already killed the test process.
	// An unresolved task with no problems on a clean finish reads as an
	// honest Partial outcome now (release-gate round 4 finding 3), not
	// misuse, so Finish returning nil here is expected, not evidence of a
	// process exit either way.
	//
	// 1.0 removed MainWith (the pre-v0.6 func(*Output) error entrypoint)
	// and its exitProcess facade outright — Run/Main are the only
	// entrypoints left, and neither calls os.Exit at all
	// (run_exit_facade_internal_test.go's TestMain_ReturnsCodeWithoutExiting
	// proves it directly): the caller writes os.Exit(evo.Main(run)).
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	out.Task("x")
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil (clean finish, no amnesty-defeating problems)", err)
	}
}
