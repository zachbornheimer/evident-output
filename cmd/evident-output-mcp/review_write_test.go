package main

import (
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// TestReviewNextAction_PartialNeverReportsClean pins commit 1855e17's
// intended fix, which reviewNextAction did not actually implement: a
// Result with Partial=true means some rule family (the removed-name
// analyzers, when their scratch module can't resolve) never ran at all, so
// "0 findings" from the rules that DID run says nothing about the ones
// that didn't. AGENTS.md's MUST-loop tells agents to stop when
// next_action is the clean sentinel — reviewNextAction must never return
// it while Partial is true, however many findings came back.
func TestReviewNextAction_PartialNeverReportsClean(t *testing.T) {
	res := review.Result{Findings: nil, RecheckRequired: false, Partial: true}
	if got := reviewNextAction(res, false); got == nextActionClean {
		t.Errorf("Partial=true with 0 findings must not report %q, got %q", nextActionClean, got)
	}
}

// TestReviewNextAction_CleanWhenNotPartial guards the ordinary case this
// change must not break: 0 findings, no recheck, and a complete (non-
// partial) review really is clean.
func TestReviewNextAction_CleanWhenNotPartial(t *testing.T) {
	res := review.Result{Findings: nil, RecheckRequired: false, Partial: false}
	if got := reviewNextAction(res, false); got != nextActionClean {
		t.Errorf("complete review with 0 findings must report %q, got %q", nextActionClean, got)
	}
}
