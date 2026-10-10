package guards_test

import (
	"strings"
	"testing"
	"unicode"

	"github.com/zachbornheimer/evident-output/mcp/internal/apisurface"
)

// satisfactionEvidence is the 1.1 allowlist of exported identifiers that
// contain "Evidence" and mean state-proof, not retained stdout/stderr.
var satisfactionEvidence = map[string]struct{}{
	"EvidencePhase": {},
	"TaskEvidence":  {},
}

func TestExportedEvidenceNamesAreSatisfactionMeaning(t *testing.T) {
	t.Parallel()
	live, err := apisurface.Walk(".")
	if err != nil {
		t.Fatal(err)
	}
	var unexpected []string
	seen := map[string]struct{}{}
	for _, line := range live {
		for _, ident := range exportedIdents(line) {
			if !strings.Contains(ident, "Evidence") {
				continue
			}
			if _, ok := satisfactionEvidence[ident]; ok {
				seen[ident] = struct{}{}
				continue
			}
			unexpected = append(unexpected, line+": "+ident)
		}
	}
	if len(unexpected) > 0 {
		t.Fatalf("capture-meaning Evidence identifier still exported (ZYS-1185):\n  %s", strings.Join(unexpected, "\n  "))
	}
	for name := range satisfactionEvidence {
		if _, ok := seen[name]; !ok {
			t.Errorf("allowlisted satisfaction name %s missing from live surface", name)
		}
	}
}

func exportedIdents(line string) []string {
	var out []string
	start := -1
	for i, r := range line {
		if isIdentRune(r, start < 0) {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			tok := line[start:i]
			if tok != "" && unicode.IsUpper(rune(tok[0])) {
				out = append(out, tok)
			}
			start = -1
		}
	}
	if start >= 0 {
		tok := line[start:]
		if tok != "" && unicode.IsUpper(rune(tok[0])) {
			out = append(out, tok)
		}
	}
	return out
}

func isIdentRune(r rune, first bool) bool {
	if first {
		return unicode.IsLetter(r) || r == '_'
	}
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}
