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

func TestGetKnownAndMissing(t *testing.T) {
	found, missing := catalog.Get([]string{"common-api", "nope"})
	if len(found) != 1 || found[0].ID != "common-api" {
		t.Fatalf("found=%#v", found)
	}
	if len(missing) != 1 || missing[0] != "nope" {
		t.Fatalf("missing=%#v", missing)
	}
}
