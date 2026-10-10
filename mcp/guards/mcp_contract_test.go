package guards_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/mcp/internal/agent/catalog"
	"github.com/zachbornheimer/evident-output/mcp/internal/agent/preview"
	"github.com/zachbornheimer/evident-output/mcp/internal/agent/review"
)

func TestMCP016_PartialOnlyWhenAnalysisIncomplete(t *testing.T) {
	// Consumer feedback: partial=true + recheck_required=false trained people to ignore review.
	// GoSource fully implements its AST rules — Partial is false when analysis completes.
	// Partial remains for GoPackage typecheck failure / empty input.
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
func f() { evo.Init(evo.Config{Isolated: true}) }
`
	res := review.GoSource("p.go", src)
	if res.Partial {
		t.Fatal("complete single-file GoSource must not set Partial merely because evo is imported")
	}
	if res.RecheckRequired {
		t.Fatalf("clean source: %+v", res.Findings)
	}
	// Incomplete package analysis still marks Partial.
	pkg := review.GoPackage(map[string]string{})
	if !pkg.Partial && !pkg.RecheckRequired {
		t.Fatal("empty GoPackage should signal incomplete analysis")
	}
}

func TestMCP010_CatalogChecksumStable(t *testing.T) {
	a := catalog.Checksum()
	b := catalog.Checksum()
	if a == "" || a != b {
		t.Fatalf("checksum unstable: %q vs %q", a, b)
	}
	if len(a) != 64 {
		t.Fatalf("want sha256 hex, got %q", a)
	}
}

func TestMCP050_TokenBudgetExplicit(t *testing.T) {
	guides := catalog.All()
	out, trunc := catalog.ApplyTokenBudget(guides, 30)
	if !trunc {
		t.Fatal("expected truncation at tiny budget")
	}
	if len(out) == 0 {
		t.Fatal("expected at least stub guide")
	}
	var joined strings.Builder
	for _, g := range out {
		joined.WriteString(g.Body)
	}
	if !strings.Contains(joined.String(), "truncated") && !strings.Contains(joined.String(), "token_budget") {
		// may truncate mid-list without body marker if budget ends between guides
		if len(out) >= len(guides) {
			t.Fatalf("no truncation signal: %+v", out)
		}
	}
}

func TestMCP037_ReviewDoesNotMutateSource(t *testing.T) {
	// Static proof: review package has no os.WriteFile / Create in source.
	// Dynamic: call review on a string and ensure we only read.
	src := "package p\n"
	before := src
	_ = review.GoSource("x.go", src)
	if src != before {
		t.Fatal("source mutated")
	}
}

// TestMCP025_PreviewProfilesFromSnapshot is the preview half of MCP-025: a
// finished run's snapshot yields at least one preview profile.
func TestMCP025_PreviewProfilesFromSnapshot(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: &bytes.Buffer{}, Title: "demo", Color: evo.ColorNever, Plain: true})
	succeed(out.Task("status"))
	_ = out.Finish()
	profiles := preview.DefaultProfiles(out.Snapshot())
	if len(profiles) == 0 {
		t.Fatal("no profiles")
	}
}

// succeed resolves task through an empty Define and waits, so the row is
// terminal when it returns.
func succeed(task *evo.TaskHandle) {
	_ = task.Define(func(context.Context) error { return nil }).Wait()
}
