package engine

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
	out := Init(Config{Isolated: true, Plain: true, Color: ColorNever, Stdout: &buf, Stderr: io.Discard})
	alpha := out.Sequence("alpha")
	alpha.Task("prune").Define(effectOf(EffectDelete, "branch", 1))
	beta := out.Sequence("beta")
	beta.Task("prune").Define(effectOf(EffectDelete, "branch", 3))
	_ = out.Close()

	rendered := buf.String()
	for _, want := range []string{"[changed] alpha › prune  deleted 1 branch", "[changed] beta › prune   deleted 3 branches"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("missing %q in:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "deleted 4 branches") {
		t.Errorf("same-named Tasks merged into one ledger row:\n%s", rendered)
	}
	changes := out.Snapshot().Changes
	if len(changes) != 2 {
		t.Fatalf("Changes = %+v, want one section per Task", changes)
	}
}

// TestLedgerNamesAnUnambiguousNestedTaskBare proves qualification is only
// for ambiguity: a nested Task whose name no other section shares keeps its
// bare name.
func TestLedgerNamesAnUnambiguousNestedTaskBare(t *testing.T) {
	var buf strings.Builder
	out := Init(Config{Isolated: true, Plain: true, Color: ColorNever, Stdout: &buf, Stderr: io.Discard})
	out.Sequence("alpha").Task("prune").Define(effectOf(EffectDelete, "branch", 2))
	_ = out.Close()
	if want := "[changed] prune  deleted 2 branches"; !strings.Contains(buf.String(), want) {
		t.Fatalf("missing %q in:\n%s", want, buf.String())
	}
}

// TestLedgerQualifiesEachSameNamedSectionOnce proves opening a section
// names only that section (and, when it first makes a name ambiguous, the
// one section already holding it): a Task's container path is fixed at
// declaration, so recomputing every earlier same-named section's subject
// on each open made n same-named sections cost O(n²) under o.mu.
func TestLedgerQualifiesEachSameNamedSectionOnce(t *testing.T) {
	out := Init(Config{Isolated: true, Stdout: io.Discard, Stderr: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	for _, repo := range []string{"repo 0", "repo 1", "repo 2"} {
		out.Group(repo).Task("prune")
	}
	out.mu.Lock()
	defer out.mu.Unlock()
	var opened []*ledgerSection
	for _, st := range out.tasks {
		if st.name == "prune" {
			opened = append(opened, out.ledgerSectionLocked(st, tenseChanged))
		}
		if len(opened) == 2 {
			for _, s := range opened {
				s.subject = "already qualified"
			}
		}
	}
	if len(opened) != 3 {
		t.Fatalf("opened %d sections, want 3", len(opened))
	}
	for _, s := range opened[:2] {
		if s.subject != "already qualified" {
			t.Errorf("earlier section recomputed to %q on a later open", s.subject)
		}
	}
	if want := "repo 2 › prune"; opened[2].subject != want {
		t.Errorf("new section subject = %q, want %q", opened[2].subject, want)
	}
}
