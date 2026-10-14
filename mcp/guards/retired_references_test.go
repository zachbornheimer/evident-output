package guards_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/rules"
)

// traceabilityRow matches one requirement row of conformance/TRACEABILITY.md:
// ID, first milestone, Test(s), Status.
var traceabilityRow = regexp.MustCompile(`^\|\s*([A-Z0-9]+-\d{3})\s*\|[^|]*\|([^|]*)\|\s*(\w+)\s*\|`)

// retiredRuleIDPattern is a word-bounded match for one retired rule ID.
func retiredRuleIDPattern(id string) *regexp.Regexp {
	return regexp.MustCompile(`\b` + regexp.QuoteMeta(id) + `\b`)
}

// TestNoStaleRetiredRuleReferences keeps retired rules and removed methods
// out of live guidance: a rule's text never cites a retired rule ID or
// teaches a removed method outside a "removed in <release>" note; every
// TRACEABILITY.md row that carries a retired rule ID is waived as retired,
// not passing on the strength of a test that no longer exists; and the live
// renderer's comments cite no retired rule ID.
func TestNoStaleRetiredRuleReferences(t *testing.T) {
	retired := rules.RetiredRules()
	if len(retired) == 0 {
		t.Fatal("rules.RetiredRules() is empty")
	}

	t.Run("rule texts", func(t *testing.T) {
		for _, r := range rules.All() {
			fields := map[string]string{
				"Invariant": r.Invariant, "Why": r.Why, "BadCode": r.BadCode, "GoodCode": r.GoodCode,
				"Remediation": r.Remediation, "Exceptions": strings.Join(r.Exceptions, "\n"),
			}
			for field, text := range fields {
				for _, rr := range retired {
					if retiredRuleIDPattern(rr.ID).MatchString(text) {
						t.Errorf("%s %s cites retired rule %s", r.ID, field, rr.ID)
					}
				}
				for _, hit := range rules.UnexplainedRetired(text) {
					t.Errorf("%s %s names removed %q without \"removed in %s\"; use %s",
						r.ID, field, hit.Match, hit.Symbol.RemovedIn, hit.Symbol.Replacement)
				}
			}
		}
	})

	t.Run("TRACEABILITY.md", func(t *testing.T) {
		body, err := os.ReadFile(filepath.Join(moduleRoot(t), "conformance", "TRACEABILITY.md"))
		if err != nil {
			t.Fatal(err)
		}
		for line := range strings.SplitSeq(string(body), "\n") {
			m := traceabilityRow.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			id, tests, status := m[1], m[2], m[3]
			if _, isRetired := rules.RetiredRuleByID(id); !isRetired {
				continue
			}
			if status != "waived" || !strings.Contains(tests, "retired") {
				t.Errorf("TRACEABILITY.md row %s is a retired rule but reads %q / %q; waive it as retired in 2.0 and cite mcp/internal/agent/rules/retired_rules.go",
					id, strings.TrimSpace(tests), status)
			}
		}
	})

	t.Run("live renderer comments", func(t *testing.T) {
		files, err := filepath.Glob(filepath.Join(moduleRoot(t), "internal", "project", "live", "*.go"))
		if err != nil || len(files) == 0 {
			t.Fatalf("no live renderer sources found: %v", err)
		}
		for _, file := range files {
			body, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			for i, line := range strings.Split(string(body), "\n") {
				for _, rr := range retired {
					if retiredRuleIDPattern(rr.ID).MatchString(line) {
						rel, _ := filepath.Rel(moduleRoot(t), file)
						t.Errorf("%s:%d cites retired rule %s: %s", rel, i+1, rr.ID, strings.TrimSpace(line))
					}
				}
			}
		}
	})
}
