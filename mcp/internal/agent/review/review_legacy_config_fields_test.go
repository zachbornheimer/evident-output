package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
)

// TestStandaloneConfigFieldsFollowTheGuide proves a v1.0.0 Config literal
// that is not an Init argument gets the guide row for each field
// (docs/migration/2.0.md, "Construction"): a rename or a lone Facades/State
// field is a rewrite, and a field 2.0 has no field for names where it goes.
func TestStandaloneConfigFieldsFollowTheGuide(t *testing.T) {
	const src = `package main

import evo "github.com/zachbornheimer/evident-output"

var cfg = evo.Config{Title: "t", StateDir: "/s", Preview: true, Isolated: true, Color: evo.ColorNever, FailedExitCode: 1, Subject: "repo"}

var streams = evo.Config{Stdout: nil, Stderr: nil}
`
	want := []struct{ message, suggestion string }{
		{"Config.State.Dir", `replace StateDir: "/s" with State: evo.State{Dir: "/s"}`},
		{"Config.DryRun", "replace Preview: true with DryRun: true"},
		{"Config.Isolated was removed in 2.0", "remove the field; nothing replaces it"},
		{"NO_COLOR", "set NO_COLOR or FORCE_COLOR"},
		{"exits 2", "map exit code 2 in the caller"},
		{"Run.Fact", `r.Fact("subject", "repo")`},
		{"Config.Facades.Stdout", "group the Facades fields of this Config into one Facades: evo.Facades{Stdout: ...}"},
		{"Config.Facades.Stderr", "group the Facades fields of this Config into one Facades: evo.Facades{Stderr: ...}"},
	}
	findings := review.GoSource("main.go", src).Findings
	for _, w := range want {
		var found bool
		for _, f := range findings {
			found = found || strings.Contains(f.Message, w.message) && strings.Contains(f.Suggestion, w.suggestion)
		}
		if !found {
			t.Errorf("no finding about %q suggesting %q:\n%s", w.message, w.suggestion, describeFindings(findings))
		}
	}
}
