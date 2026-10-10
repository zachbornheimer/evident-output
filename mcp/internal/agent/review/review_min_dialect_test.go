package review

import (
	"strconv"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/internal/agent/rules"
)

// pinBelow is a desired_version one minor release older than minDialect.
func pinBelow(t *testing.T, minDialect string) string {
	t.Helper()
	v, ok := parseSemver3(minDialect)
	if !ok || v[1] == 0 && v[0] == 0 {
		t.Fatalf("MinDialect %q is not a releasable x.y.z", minDialect)
	}
	if v[1] > 0 {
		return "v" + strconv.Itoa(v[0]) + "." + strconv.Itoa(v[1]-1) + ".99"
	}
	return "v" + strconv.Itoa(v[0]-1) + ".99.99"
}

// evoFile wraps a rule's BadCode in a file that imports evo.
func evoFile(body string) string {
	return "package main\n\nimport (\n\t\"context\"\n\tevo \"github.com/zachbornheimer/evident-output\"\n)\n\nvar _ = context.Background\nvar _ = evo.Init\n\nfunc example(ctx context.Context, task *evo.TaskHandle) {\n" + body + "\n}\n"
}

// TestRuleMinDialectGovernsFiring proves a rule's catalog MinDialect alone
// decides the pins it fires for: its BadCode is never reported one release
// below MinDialect, and wherever it is reported, the finding's
// RequiredVersion is exactly MinDialect.
func TestRuleMinDialectGovernsFiring(t *testing.T) {
	fired := 0
	for _, r := range rules.All() {
		if r.MinDialect == "" || r.BadCode == "" {
			continue
		}
		src := evoFile(r.BadCode)
		for _, f := range GoSourceAt("bad.go", src, pinBelow(t, r.MinDialect)).Findings {
			if f.RuleID == r.ID {
				t.Errorf("%s (MinDialect %s) fired for pin %s", r.ID, r.MinDialect, pinBelow(t, r.MinDialect))
			}
		}
		for _, f := range GoSourceAt("bad.go", src, "").Findings {
			if f.RuleID != r.ID {
				continue
			}
			fired++
			if f.RequiredVersion != r.MinDialect {
				t.Errorf("%s finding RequiredVersion = %q, want MinDialect %q", r.ID, f.RequiredVersion, r.MinDialect)
			}
		}
	}
	if fired == 0 {
		t.Fatal("no MinDialect rule's BadCode fired at the current dialect; the test proves nothing")
	}
	t.Logf("%d MinDialect findings fired at the current dialect", fired)
}
