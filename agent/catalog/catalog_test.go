package catalog_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/agent/catalog"
	"github.com/zachbornheimer/evident-output/agent/rules"
)

func TestFilterByUseCase(t *testing.T) {
	got := catalog.Filter("progress")
	if len(got) == 0 {
		t.Fatal("expected progress guides")
	}
	found := false
	for _, g := range got {
		if g.ID == "tasks" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected tasks guide, got %#v", got)
	}
}

func TestFilterFirstPaintByUseCase(t *testing.T) {
	for _, uc := range []string{"startup", "latency", "blank", "streaming"} {
		got := catalog.Filter(uc)
		found := false
		for _, g := range got {
			if g.ID == "first-paint" {
				found = true
			}
		}
		if !found {
			t.Fatalf("use case %q: expected first-paint guide, got %#v", uc, got)
		}
	}
}

func TestFirstPaintGuideCarriesFPRules(t *testing.T) {
	found, missing := catalog.Get([]string{"first-paint"})
	if len(missing) != 0 || len(found) != 1 {
		t.Fatalf("found=%#v missing=%#v", found, missing)
	}
	g := found[0]
	for _, want := range []string{"FP-001", "FP-002", "FP-003"} {
		hit := false
		for _, r := range g.Rules {
			if r == want {
				hit = true
			}
		}
		if !hit {
			t.Fatalf("first-paint guide missing rule %s: %#v", want, g.Rules)
		}
	}
}

// TestPhaseQRuleIDsResolve proves the rule IDs this work order's item F added
// to guide Rules lists resolve through rules.Explain — a typo'd reference is
// a dead end identical to the review-emitted-ID gap
// TestReviewEmittedIDsAreRegistered closes on the other side. Scoped to the
// IDs added here, not the full catalog: several pre-existing guide Rules
// entries (API-001, DOM-006/007/016/017, OUT-001/003/004, TXT-007, SEC-006,
// TERM-006, LOG-001) predate this work order and are out of its blast radius.
func TestPhaseQRuleIDsResolve(t *testing.T) {
	for _, id := range []string{"BOUND-001", "API-030", "API-031", "CONFIRM-002", "CON-002", "FP-004"} {
		if _, ok := rules.Explain(id); !ok {
			t.Errorf("rule %s referenced by a guide cannot be resolved by rules.Explain", id)
		}
	}
}

// TestGuidesCoverPhaseQAdditions pins evo-rec.md work order item F: the
// guidance catalog must teach bounded Because/Detail text, predeclare-
// before-fan-out, PhaseWriter over hand-rolled writers, and Destructive()
// on destructive confirms.
func TestGuidesCoverPhaseQAdditions(t *testing.T) {
	all := catalog.All()
	bodyContains := func(needle string) bool {
		for _, g := range all {
			if strings.Contains(g.Body, needle) {
				return true
			}
		}
		return false
	}
	for _, want := range []string{"TruncateNames", "Predeclare before fan-out", "PhaseWriter", "Destructive()"} {
		if !bodyContains(want) {
			t.Errorf("no guide body mentions %q", want)
		}
	}
	ruleCovered := func(id string) bool {
		for _, g := range all {
			for _, r := range g.Rules {
				if r == id {
					return true
				}
			}
		}
		return false
	}
	for _, id := range []string{"BOUND-001", "API-030", "API-031", "CONFIRM-002"} {
		if !ruleCovered(id) {
			t.Errorf("no guide lists rule %s", id)
		}
	}
}

func TestGetKnownAndMissing(t *testing.T) {
	found, missing := catalog.Get([]string{"common-api", "nope"})
	if len(found) != 1 || found[0].ID != "common-api" {
		t.Fatalf("found=%#v", found)
	}
	if len(missing) != 1 || missing[0] != "nope" {
		t.Fatalf("missing=%#v", missing)
	}
}
