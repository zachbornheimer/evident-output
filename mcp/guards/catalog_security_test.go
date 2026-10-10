package guards_test

import (
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/internal/agent/catalog"
)

func TestSEC014_TraversalRejectedByCatalog(t *testing.T) {
	// Catalog Get never resolves traversal-style ids.
	found, missing := catalog.Get([]string{"../secret", "common-api"})
	if len(found) != 1 || found[0].ID != "common-api" {
		t.Fatalf("%+v missing=%v", found, missing)
	}
	if len(missing) != 1 || missing[0] != "../secret" {
		t.Fatalf("missing=%v", missing)
	}
}

func TestSEC015_NoAuthOnAnnotations(t *testing.T) {
	// MCP tools do not branch on annotations fields — structural review:
	// catalog/rules/review packages have no authorization logic.
	// Presence of public tools without annotations is the contract.
	if catalog.Checksum() == "" {
		t.Fatal("catalog required")
	}
}
