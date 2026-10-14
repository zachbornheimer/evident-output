package guards_test

import (
	"strings"
	"testing"
	"unicode"

	"github.com/zachbornheimer/evident-output/v2/mcp/internal/apisurface"
)

// TestNoExportedEvidenceNames pins ZYS-1185 through 2.0: capture is spelled
// Capture, and 2.0 removed the satisfaction-meaning EvidencePhase and
// TaskEvidence too, so no exported identifier contains "Evidence".
func TestNoExportedEvidenceNames(t *testing.T) {
	t.Parallel()
	live, err := apisurface.Walk(".")
	if err != nil {
		t.Fatal(err)
	}
	var unexpected []string
	for _, line := range live {
		for _, ident := range exportedIdents(line) {
			if !strings.Contains(ident, "Evidence") {
				continue
			}
			unexpected = append(unexpected, line+": "+ident)
		}
	}
	if len(unexpected) > 0 {
		t.Fatalf("Evidence identifier still exported (ZYS-1185, removed in 2.0):\n  %s", strings.Join(unexpected, "\n  "))
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
