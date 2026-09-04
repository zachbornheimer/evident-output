package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestUnchanged_AllTasksUnchanged_ConcludesStateUnchanged is I7: a run made
// entirely of Task.Unchanged resolutions concludes StateUnchanged instead
// of the generic StateReady an ordinary Done gets.
func TestUnchanged_AllTasksUnchanged_ConcludesStateUnchanged(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	out.Task("config").Unchanged("already up to date")
	out.Task("lockfile").Unchanged("no drift (%d deps checked)", 42)

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if got := out.Conclusion().State; got != evo.StateUnchanged {
		t.Fatalf("conclusion state = %q, want %q", got, evo.StateUnchanged)
	}
}

// TestUnchanged_MixedWithOrdinaryDone_StaysReady proves a single ordinary
// Done alongside an Unchanged task keeps the generic StateReady verdict —
// Unchanged only wins when every Done-family task agrees.
func TestUnchanged_MixedWithOrdinaryDone_StaysReady(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	out.Task("config").Unchanged("already up to date")
	out.Task("cache").Done("warmed")

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if got := out.Conclusion().State; got != evo.StateReady {
		t.Fatalf("conclusion state = %q, want %q", got, evo.StateReady)
	}
	if !strings.Contains(buf.String(), "already up to date") {
		t.Fatalf("expected the Unchanged summary rendered, got:\n%s", buf.String())
	}
}
