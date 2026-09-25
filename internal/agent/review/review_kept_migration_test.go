package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// API-100 and API-101: Kept and ForSkip are removed in 1.1 with no alias.
// A policy-excluded per-candidate Task is Skipped with an evo.Reason; a
// count such as "kept 383" is a Fact or part of the Summary.

const keptPerItemSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
var protected = evo.Reason("protected")
func run(branches *evo.GroupHandle, names []string) {
  for _, name := range names {
    branches.Task(name).Kept(protected)
  }
}
`

func TestAPI100_KeptOnTask_FiresWithSkippedRewrite(t *testing.T) {
	res := review.GoSource("prune.go", keptPerItemSrc)
	f := findingByID(t, res, "API-100")
	if !strings.Contains(f.Suggestion, "branches.Task(name).Skipped(protected)") {
		t.Fatalf("API-100 suggestion does not spell the Skipped rewrite: %q", f.Suggestion)
	}
	if !strings.Contains(f.Suggestion, "Fact") {
		t.Fatalf("API-100 suggestion does not route a kept count to a Fact: %q", f.Suggestion)
	}
	if f.RequiredVersion != "1.1.0" {
		t.Fatalf("API-100 required_version = %q, want 1.1.0", f.RequiredVersion)
	}
}

// A Kept method on anything that is not an evo Task is not this rule's
// target.
const keptOnOtherTypeSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
type ledger struct{}
func (ledger) Kept(string) {}
func run(l ledger) {
  _ = evo.Reason("x")
  l.Kept("main")
}
`

func TestAPI100_KeptOnOtherType_Silent(t *testing.T) {
	res := review.GoSource("ledger.go", keptOnOtherTypeSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-100" {
			t.Fatalf("API-100 fired on a non-evo Kept method: %+v", f)
		}
	}
}

const forSkipSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func opts() []evo.ReasonOption {
  return []evo.ReasonOption{evo.ForSkip(), evo.OnTask("branches")}
}
`

func TestAPI101_ForSkip_Fires(t *testing.T) {
	res := review.GoSource("reasons.go", forSkipSrc)
	f := findingByID(t, res, "API-101")
	if !strings.Contains(f.Suggestion, "delete") {
		t.Fatalf("API-101 suggestion does not say to delete the option: %q", f.Suggestion)
	}
}

// A caller that imports evident-output under a non-"evo" alias still calls
// the removed ForSkip — the detector must resolve the alias the way API-100
// resolves Task/evo idents, not hardcode the "evo." spelling.
const forSkipAliasedSrc = `package p
import eo "github.com/zachbornheimer/evident-output"
func opts() []eo.ReasonOption {
  return []eo.ReasonOption{eo.ForSkip(), eo.OnTask("branches")}
}
`

func TestAPI101_ForSkip_FiresUnderImportAlias(t *testing.T) {
	res := review.GoSource("reasons_aliased.go", forSkipAliasedSrc)
	f := findingByID(t, res, "API-101")
	if !strings.Contains(f.Suggestion, "delete") {
		t.Fatalf("API-101 suggestion does not say to delete the option: %q", f.Suggestion)
	}
}

// A caller with no evident-output import at all — a local ForSkip-named
// function of its own — is not this rule's target.
const forSkipUnrelatedSrc = `package p
func ForSkip() string { return "x" }
func run() string { return ForSkip() }
`

func TestAPI101_ForSkip_SilentWithNoEvoImport(t *testing.T) {
	res := review.GoSource("unrelated.go", forSkipUnrelatedSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-101" {
			t.Fatalf("API-101 fired without an evident-output import: %+v", f)
		}
	}
}

// A 1.0 pin never had Kept/ForSkip removed, so the migration rules must
// stay silent there (docs/migration/1.1.md: "a 1.0 pin is not flagged").
func TestAPI100And101_SilentOnDesiredVersion1_0(t *testing.T) {
	res := review.GoSourceAt("prune_v10.go", keptPerItemSrc, "1.0.0")
	for _, f := range res.Findings {
		if f.RuleID == "API-100" || f.RuleID == "API-101" {
			t.Fatalf("rule %s fired on a 1.0 pin: %+v", f.RuleID, f)
		}
	}

	res = review.GoSourceAt("reasons_v10.go", forSkipSrc, "1.0.0")
	for _, f := range res.Findings {
		if f.RuleID == "API-100" || f.RuleID == "API-101" {
			t.Fatalf("rule %s fired on a 1.0 pin: %+v", f.RuleID, f)
		}
	}
}
