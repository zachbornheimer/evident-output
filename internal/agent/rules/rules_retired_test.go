package rules

import (
	"testing"

	"github.com/zachbornheimer/evident-output/internal/retired"
)

// TestCatalogNeverTeachesARetiredSymbol fails when a rule's GoodCode, or
// its Invariant/Remediation outside a "removed in <release>" note, names a
// retired API. The review autofixer's contract is "apply every
// suggestion"; guidance that names a removed API breaks the build.
func TestCatalogNeverTeachesARetiredSymbol(t *testing.T) {
	for _, r := range All() {
		for _, h := range retired.TaughtIn(r.GoodCode) {
			t.Errorf("%s GoodCode teaches retired %q; use %s", r.ID, h.Match, h.Symbol.Replacement)
		}
		for field, text := range map[string]string{"Invariant": r.Invariant, "Remediation": r.Remediation} {
			for _, h := range retired.UnexplainedIn(text) {
				t.Errorf("%s %s names retired %q without \"removed in %s\"; use %s", r.ID, field, h.Match, h.Symbol.RemovedIn, h.Symbol.Replacement)
			}
		}
	}
	for _, m := range Migrations() {
		for _, h := range retired.TaughtIn(m.To) {
			t.Errorf("migration %q -> %q: To teaches retired %q", m.From, m.To, h.Match)
		}
	}
}
