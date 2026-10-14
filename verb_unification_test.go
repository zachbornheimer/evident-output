package evo

import (
	"bytes"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/internal/change"
)

// TestVerbVocabulary_UnifiedAcrossMutationVerbsAndRunModes is C10: every
// Task mutation verb (Add/Create/Update/Remove/Delete/Write/Push)
// shares one call shape and conjugates consistently — imperative under
// DryRun's Plan ledger, past tense on the applied Changes ledger. P1
// deleted the standalone Output.Changes/Output.Plan entry points (and their
// past-tense builder methods), so this now proves the same unification
// through Task alone, across both run modes, instead of across three
// separate types.
func TestVerbVocabulary_UnifiedAcrossMutationVerbsAndRunModes(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})

	cleanup := out.Task("cleanup")
	cleanup.Define(effectOf(Verb(change.EffectAdd), "worktree", 2))

	applied := out.Task("changes-section")
	applied.Define(effectOf(Delete, "branch", 1))

	pushed := out.Task("tags")
	pushed.Define(effectOf(Verb(change.EffectPush), "tag", 1))

	_ = out.finish()

	rendered := buf.String()
	for _, want := range []string{"add", "2 worktree", "deleted", "1 branch", "pushed", "1 tag"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected %q in rendered output, got:\n%s", want, rendered)
		}
	}
}

// TestVerbVocabulary_PlanKeepsImperativeUnderDryRun is the DryRun half of
// the same C10 unification: the same Push verb call renders imperative
// ("push"), never conjugated, under Config.DryRun.
func TestVerbVocabulary_PlanKeepsImperativeUnderDryRun(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{DryRun: true, Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})

	plan := out.Task("plan-section")
	plan.Define(effectOf(Verb(change.EffectPush), "commit", 3))

	_ = out.finish()

	rendered := buf.String()
	for _, want := range []string{"push", "3 commit"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected %q in rendered output, got:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "pushed") {
		t.Fatalf("dry run must not conjugate to past tense, got:\n%s", rendered)
	}
}
