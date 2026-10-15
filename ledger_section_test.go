package evo

import (
	"io"
	"strings"
	"testing"
)

// TestLedgerKeepsSameNamedTasksInDifferentContainersApart proves a ledger
// section belongs to its Task, not its display name. Sequence "alpha"
// Task "prune" deletes 1 branch and Sequence "beta" Task "prune" deletes 3;
// keyed by name they merged into one `[changed] prune  deleted 4 branches`
// row, so neither repo's count was true. Each now gets its own row, named
// by its container path because the bare name is ambiguous.
func TestLedgerKeepsSameNamedTasksInDifferentContainersApart(t *testing.T) {
	var buf strings.Builder
	out := initRun(Config{Output: Plain, Facades: Facades{Stdout: io.Discard, Stderr: &buf}}, tuning{color: colorNever})
	alpha := out.Sequence("alpha")
	alpha.Task("prune").Define(effectOf(Delete, "branch", 1))
	beta := out.Sequence("beta")
	beta.Task("prune").Define(effectOf(Delete, "branch", 3))
	_ = out.close()

	rendered := buf.String()
	for _, want := range []string{"[changed] alpha › prune  deleted 1 branch", "[changed] beta › prune   deleted 3 branches"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("missing %q in:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "deleted 4 branches") {
		t.Errorf("same-named Tasks merged into one ledger row:\n%s", rendered)
	}
	changes := out.snapshot().Changes
	if len(changes) != 2 {
		t.Fatalf("Changes = %+v, want one section per Task", changes)
	}
}

// TestLedgerNamesAnUnambiguousNestedTaskBare proves qualification is only
// for ambiguity: a nested Task whose name no other section shares keeps its
// bare name.
func TestLedgerNamesAnUnambiguousNestedTaskBare(t *testing.T) {
	var buf strings.Builder
	out := initRun(Config{Output: Plain, Facades: Facades{Stdout: io.Discard, Stderr: &buf}}, tuning{color: colorNever})
	out.Sequence("alpha").Task("prune").Define(effectOf(Delete, "branch", 2))
	_ = out.close()
	if want := "[changed] prune  deleted 2 branches"; !strings.Contains(buf.String(), want) {
		t.Fatalf("missing %q in:\n%s", want, buf.String())
	}
}
