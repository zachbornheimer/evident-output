package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// removedFailfBlockfSrc wraps body in a run() that both calls Define (so a
// body placed inside runDefine sits in a Define-resolved callback) and
// leaves body reachable at the top level when runDefine is empty — the
// two shapes API-080/API-081 must rewrite differently (inside vs. outside
// Define, per each rule's own GoodCode in rules_api.go).
func removedFailfBlockfSrc(outsideDefine, insideDefine string) string {
	return `package p
import (
  "context"
  "fmt"
  evo "github.com/zachbornheimer/evident-output"
)
func run(task *evo.TaskHandle, err error) error {
  task.Define(func(ctx context.Context) error {
    ` + insideDefine + `
  })
  ` + outsideDefine + `
  _ = fmt.Sprint(err)
  return nil
}
`
}

// TestRemovedBlockf_OutsideDefine_ReturnsNil pins API-081's outside-Define
// GoodCode: `task.Blockf("summary: %w", err)` becomes `task.Block("summary");
// return nil` — DOM-011 flags a returned error there as the expected-
// blocked-treated-as-application-error shape, so the rewrite must drop the
// cause, not hand it back. This is the exact case commit 8584db8 claimed to
// converge but did not: the pre-fix rewrite fired DOM-011 on re-review and
// the MUST-loop never cleared.
func TestRemovedBlockf_OutsideDefine_ReturnsNil(t *testing.T) {
	src := removedFailfBlockfSrc(`return task.Blockf("worktree dirty: %w", err).NextCommand("git", "status")`, "")
	res := review.GoSource("blockf.go", src)

	var hits []review.Finding
	for _, f := range res.Findings {
		if f.RuleID == "API-081" {
			hits = append(hits, f)
		}
	}
	if len(hits) != 1 {
		t.Fatalf("want one API-081 finding, got %d: %+v", len(hits), res.Findings)
	}
	suggestion := hits[0].Suggestion
	if !strings.Contains(suggestion, "return nil") {
		t.Fatalf("outside-Define suggestion = %q, want `return nil` (DOM-011's outside-Define shape)", suggestion)
	}
	if strings.Contains(suggestion, "return err") {
		t.Fatalf("outside-Define suggestion = %q, must not hand the cause back as a returned error (fires DOM-011)", suggestion)
	}
	if !strings.Contains(suggestion, `evo.NextCommand("git", "status")`) {
		t.Fatalf("outside-Define suggestion = %q, want the .NextCommand chain folded into a Block ProblemOption", suggestion)
	}

	// The rewrite must actually converge: re-reviewing the suggested
	// source fires neither API-081 again nor DOM-011.
	rewritten := removedFailfBlockfSrc(suggestion, "")
	for _, f := range review.GoSource("blockf.go", rewritten).Findings {
		if f.RuleID == "API-081" || f.RuleID == "DOM-011" {
			t.Fatalf("applied suggestion still flags %s: %+v", f.RuleID, f)
		}
	}
}

// TestRemovedBlockf_InsideDefine_ReturnsCause pins API-081's inside-Define
// GoodCode: the returned cause stays, because Define needs it to resolve
// the task, and DOM-011 exempts this exact shape (Block resolves Blocked;
// the return only lets Define propagate the cause).
func TestRemovedBlockf_InsideDefine_ReturnsCause(t *testing.T) {
	src := removedFailfBlockfSrc("", `if err != nil {
      return task.Blockf("worktree dirty: %w", err)
    }
    return nil`)
	res := review.GoSource("blockf.go", src)

	var hits []review.Finding
	for _, f := range res.Findings {
		if f.RuleID == "API-081" {
			hits = append(hits, f)
		}
	}
	if len(hits) != 1 {
		t.Fatalf("want one API-081 finding, got %d: %+v", len(hits), res.Findings)
	}
	suggestion := hits[0].Suggestion
	if !strings.Contains(suggestion, "return err") {
		t.Fatalf("inside-Define suggestion = %q, want the cause returned so Define resolves the task", suggestion)
	}
	if strings.Contains(suggestion, "return nil") {
		t.Fatalf("inside-Define suggestion = %q, must not discard the cause Define needs", suggestion)
	}

	rewritten := removedFailfBlockfSrc("", `if err != nil {
      `+suggestion+`
    }
    return nil`)
	for _, f := range review.GoSource("blockf.go", rewritten).Findings {
		if f.RuleID == "API-081" || f.RuleID == "DOM-011" {
			t.Fatalf("applied suggestion still flags %s: %+v", f.RuleID, f)
		}
	}
}

// TestRemovedBlockf_NextChain_MultiArgSplit pins the multi-arg
// `.Next(a, b)` rewrite: evo.Next(action) takes a single Action (the
// removed *Failure.Next was variadic), so each argument becomes its own
// evo.Next(...) ProblemOption rather than one evo.Next call holding both
// (which would not compile).
func TestRemovedBlockf_NextChain_MultiArgSplit(t *testing.T) {
	src := removedFailfBlockfSrc(`return task.Blockf("worktree dirty: %w", err).Next(remedyA, remedyB)`, "")
	res := review.GoSource("blockf.go", src)

	var hit *review.Finding
	for _, f := range res.Findings {
		if f.RuleID == "API-081" {
			hit = &f
			break
		}
	}
	if hit == nil {
		t.Fatalf("want an API-081 finding, got: %+v", res.Findings)
	}
	if !strings.Contains(hit.Suggestion, "evo.Next(remedyA), evo.Next(remedyB)") {
		t.Fatalf("suggestion = %q, want each .Next argument split into its own evo.Next(...) option", hit.Suggestion)
	}
}

// TestRemovedFailf_InsideVsOutsideDefine pins API-080's own Define split
// (the Failf loop's existing behavior, guarded here so it cannot regress
// alongside the Blockf fix above): inside Define the rewrite drops the Fail
// call entirely and returns fmt.Errorf directly (Define resolves the task
// from the returned error); outside Define it keeps a statement-form Fail
// and returns the cause.
func TestRemovedFailf_InsideVsOutsideDefine(t *testing.T) {
	outsideSrc := removedFailfBlockfSrc(`return task.Failf("validate policy manifest: %w", err)`, "")
	outsideRes := review.GoSource("failf.go", outsideSrc)
	var outsideHit *review.Finding
	for _, f := range outsideRes.Findings {
		if f.RuleID == "API-080" {
			outsideHit = &f
			break
		}
	}
	if outsideHit == nil {
		t.Fatalf("want an API-080 finding outside Define, got: %+v", outsideRes.Findings)
	}
	if !strings.Contains(outsideHit.Suggestion, "task.Fail(") || !strings.Contains(outsideHit.Suggestion, "return err") {
		t.Fatalf("outside-Define suggestion = %q, want a kept Fail statement plus return err", outsideHit.Suggestion)
	}

	insideSrc := removedFailfBlockfSrc("", `if err != nil {
      return task.Failf("validate policy manifest: %w", err)
    }
    return nil`)
	insideRes := review.GoSource("failf.go", insideSrc)
	var insideHit *review.Finding
	for _, f := range insideRes.Findings {
		if f.RuleID == "API-080" {
			insideHit = &f
			break
		}
	}
	if insideHit == nil {
		t.Fatalf("want an API-080 finding inside Define, got: %+v", insideRes.Findings)
	}
	if strings.Contains(insideHit.Suggestion, "task.Fail(") {
		t.Fatalf("inside-Define suggestion = %q, must not keep a redundant Fail call (API-040)", insideHit.Suggestion)
	}
	if !strings.Contains(insideHit.Suggestion, "fmt.Errorf(") {
		t.Fatalf("inside-Define suggestion = %q, want a bare returned fmt.Errorf so Define resolves the task", insideHit.Suggestion)
	}
}

// TestRemovedFailfBlockf_BareFallback_ComputedFormat pins the
// bareFailfBlockfPattern fallback used when the call's format string is
// computed (not the cheap %w-suffixed literal shape the two cause patterns
// rewrite). Block's fallback message must match its own location, exactly
// as the derived rewrite above does: `return nil` outside Define, keep the
// returned cause inside Define — never the generic "%w-wrapped error from
// Define" text Fail's fallback correctly uses (Block's is not a Define-only
// shape).
func TestRemovedFailfBlockf_BareFallback_ComputedFormat(t *testing.T) {
	outsideSrc := removedFailfBlockfSrc(`return task.Blockf(computedFormat(), err)`, "")
	outsideRes := review.GoSource("blockf.go", outsideSrc)
	var outsideHit *review.Finding
	for _, f := range outsideRes.Findings {
		if f.RuleID == "API-081" {
			outsideHit = &f
			break
		}
	}
	if outsideHit == nil {
		t.Fatalf("want an API-081 finding outside Define, got: %+v", outsideRes.Findings)
	}
	if !strings.Contains(outsideHit.Suggestion, "return nil") {
		t.Fatalf("outside-Define bare-fallback suggestion = %q, want it to say return nil, not a %%w-wrapped error from Define", outsideHit.Suggestion)
	}

	insideSrc := removedFailfBlockfSrc("", `if err != nil {
      return task.Blockf(computedFormat(), err)
    }
    return nil`)
	insideRes := review.GoSource("blockf.go", insideSrc)
	var insideHit *review.Finding
	for _, f := range insideRes.Findings {
		if f.RuleID == "API-081" {
			insideHit = &f
			break
		}
	}
	if insideHit == nil {
		t.Fatalf("want an API-081 finding inside Define, got: %+v", insideRes.Findings)
	}
	if !strings.Contains(insideHit.Suggestion, "Define") || strings.Contains(insideHit.Suggestion, "return nil") {
		t.Fatalf("inside-Define bare-fallback suggestion = %q, want it to say return the cause from Define, not return nil", insideHit.Suggestion)
	}
}

// TestRemovedFailfBlockf_PreDialectOneOne_NotFlagged pins the version gate:
// a consumer pinned before 1.1 still has Failf/Blockf, so neither rule
// fires below dialectOneOne.
func TestRemovedFailfBlockf_PreDialectOneOne_NotFlagged(t *testing.T) {
	src := removedFailfBlockfSrc(`return task.Blockf("worktree dirty: %w", err)`, "")
	res := review.GoSourceAt("blockf.go", src, "1.0.0")
	for _, f := range res.Findings {
		if f.RuleID == "API-080" || f.RuleID == "API-081" {
			t.Fatalf("pre-1.1 pin fired %s, want it silent below dialectOneOne: %+v", f.RuleID, f)
		}
	}
}
