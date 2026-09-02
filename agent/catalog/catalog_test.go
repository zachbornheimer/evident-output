package catalog_test

import (
	"testing"

	"github.com/zachbornheimer/evident-output/agent/catalog"
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

func TestGetKnownAndMissing(t *testing.T) {
	found, missing := catalog.Get([]string{"common-api", "nope"})
	if len(found) != 1 || found[0].ID != "common-api" {
		t.Fatalf("found=%#v", found)
	}
	if len(missing) != 1 || missing[0] != "nope" {
		t.Fatalf("missing=%#v", missing)
	}
}
