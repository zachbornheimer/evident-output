package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestVerbVocabulary_UnifiedAcrossMutationVerbsAndRunModes is C10: every
// TaskHandle mutation verb (Add/Create/Update/Remove/Delete/Write/Push)
// shares one call shape and conjugates consistently — imperative under
// DryRun's Plan ledger, past tense on the applied Changes ledger. P1
// deleted the standalone Output.Changes/Output.Plan entry points (and their
// past-tense builder methods), so this now proves the same unification
// through TaskHandle alone, across both run modes, instead of across three
// separate types.
func TestVerbVocabulary_UnifiedAcrossMutationVerbsAndRunModes(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})

	cleanup := out.Task("cleanup")
	cleanup.Define(effectOf(evo.EffectAdd, "worktree", 2))

	applied := out.Task("changes-section")
	applied.Define(effectOf(evo.EffectDelete, "branch", 1))

	pushed := out.Task("tags")
	pushed.Define(effectOf(evo.EffectPush, "tag", 1))

	_ = out.Finish()

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
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true, DryRun: true})

	plan := out.Task("plan-section")
	plan.Define(effectOf(evo.EffectPush, "commit", 3))

	_ = out.Finish()

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
