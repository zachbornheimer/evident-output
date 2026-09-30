//go:build evopending

// Pending MCP pit-of-success rules (ZYS-1368, ZYS-1369). Each rule is a bad
// source that must be flagged with a finding that teaches the fix, and a good
// source that must review clean. Rule ids are not fixed yet, so a rule is
// recognised by the words its message or suggestion must contain.
package mcp_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// findingTeaching returns the first finding whose message plus suggestion
// contains every keyword (case-insensitive).
func findingTeaching(res review.Result, keywords ...string) (review.Finding, bool) {
	for _, f := range res.Findings {
		text := strings.ToLower(f.Message + " " + f.Suggestion)
		matched := true
		for _, k := range keywords {
			matched = matched && strings.Contains(text, strings.ToLower(k))
		}
		if matched {
			return f, true
		}
	}
	return review.Finding{}, false
}

func requireFlagged(t *testing.T, src string, keywords ...string) review.Finding {
	t.Helper()
	res := review.GoSource("bad.go", src)
	f, ok := findingTeaching(res, keywords...)
	if !ok {
		t.Fatalf("no finding mentions %v; findings: %+v", keywords, res.Findings)
	}
	if !res.RecheckRequired {
		t.Fatalf("a flagged composition defect must keep recheck_required=true")
	}
	return f
}

func requireClean(t *testing.T, src string) {
	t.Helper()
	res := review.GoSource("good.go", src)
	if len(res.Findings) != 0 || res.RecheckRequired {
		t.Fatalf("good source must review clean; got %+v (recheck=%v)", res.Findings, res.RecheckRequired)
	}
}

const evoImport = `package p

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

var _ = os.ReadDir
var _ context.Context
`
