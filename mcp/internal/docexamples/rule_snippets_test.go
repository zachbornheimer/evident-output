package docexamples_test

import (
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/internal/agent/rules"
	"github.com/zachbornheimer/evident-output/mcp/internal/docexamples"
)

// TestRuleGoodCodeTypeChecks extends ZYS-830's harness to the one MCP
// snippet corpus it never saw: every rule's GoodCode. TestDocFencesMatchFixtures
// only walks ```go fences in Markdown (the docs and catalog guides); a
// rule's GoodCode is a Go string field in mcp/internal/agent/rules, served by
// evident_output_explain and review suggestions, so it drifted unchecked
// (TAX-001 still showed the two-argument Skipped 1.0 removed).
func TestRuleGoodCodeTypeChecks(t *testing.T) {
	for _, r := range rules.All() {
		t.Run(r.ID, func(t *testing.T) {
			if _, prose := proseGoodCode[r.ID]; prose {
				if _, err := docexamples.TypeCheckSnippet(r.GoodCode); err == nil {
					t.Fatalf("%s GoodCode parses as Go now; drop it from proseGoodCode so it is type-checked", r.ID)
				}
				return
			}
			errs, err := docexamples.TypeCheckSnippet(r.GoodCode)
			if err != nil {
				t.Fatalf("%s GoodCode is not Go (add it to proseGoodCode only if it is deliberately not code): %v\n%s", r.ID, err, r.GoodCode)
			}
			for _, e := range errs {
				t.Errorf("%s GoodCode: %v", r.ID, e)
			}
			if len(errs) > 0 {
				t.Logf("snippet:\n%s", r.GoodCode)
			}
		})
	}
}

// proseGoodCode names the rules whose GoodCode is deliberately not Go — a
// comment-only instruction, a JSON document, a terminal transcript — and
// why. Each entry must still fail to parse as Go, so an entry cannot hide
// a real snippet.
var proseGoodCode = map[string]string{
	"SCHEMA-001": "a JSON snapshot document, not Go",
}
