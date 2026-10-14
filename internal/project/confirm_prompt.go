package project

import (
	"fmt"
	"strings"
)

// confirmInputLine is the P11 input line under the question: the typed
// answer lands there, on its own line, so the question and its choices never
// compete with the keystrokes. It ends without a newline: the answer, and
// the terminal's own newline on Enter, continue it.
const confirmInputLine = "›"

// ConfirmPrompt is the durable "?  <question>  [y/N]" gate a Confirm writes:
// the question, marked "(destructive)" when it is, each non-empty detail
// line under it, then the input line.
func ConfirmPrompt(question string, destructive bool, detail []string, style Style) string {
	if destructive {
		question += "  (destructive)"
	}
	glyph := StyleGlyph(GlyphHumanInput.Render(style.Profile), SGRCyan, style.Color)
	var b strings.Builder
	fmt.Fprintf(&b, "%s  %s  [y/N]\n", glyph, question)
	for _, line := range detail {
		if line != "" {
			fmt.Fprintf(&b, "  %s\n", Dim(line, style.Color))
		}
	}
	b.WriteString(confirmInputLine)
	return b.String()
}
