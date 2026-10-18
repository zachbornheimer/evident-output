package review_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
)

// evoBody wraps body in a package that imports the 2.0 module as evo.
func evoBody(body string) string {
	return "package p\nimport evo \"github.com/zachbornheimer/evident-output/v2\"\n" + body
}

// evoIOBody is evoBody with io and os imported too.
func evoIOBody(body string) string {
	return "package p\nimport (\n\t\"io\"\n\t\"os\"\n\tevo \"github.com/zachbornheimer/evident-output/v2\"\n)\n" + body
}

func joinSuggestions(fs []review.Finding) string {
	parts := make([]string, len(fs))
	for i, f := range fs {
		parts[i] = f.Suggestion
	}
	return strings.Join(parts, " | ")
}

func assertNoRule(t *testing.T, res review.Result, ruleID string) {
	t.Helper()
	for _, f := range res.Findings {
		if f.RuleID == ruleID {
			t.Fatalf("%s fired: %+v", ruleID, f)
		}
	}
}

// tryApplyReplace applies a one-line "replace OLD with NEW" suggestion to
// src. " with " may also occur inside a quoted summary on either side, so OLD
// is the first split that is lexically complete and occurs in src.
func tryApplyReplace(src, suggestion string) (string, bool) {
	rest, ok := strings.CutPrefix(suggestion, "replace ")
	if !ok || strings.Contains(suggestion, "\n") {
		return "", false // a Suggestion is one line (review.Finding)
	}
	for offset := 0; ; {
		i := strings.Index(rest[offset:], " with ")
		if i < 0 {
			return "", false
		}
		cut := offset + i
		old, repl := rest[:cut], rest[cut+len(" with "):]
		offset = cut + 1
		pattern, closed := leftPattern(old)
		if !closed {
			continue
		}
		if loc := regexp.MustCompile(pattern).FindStringIndex(src); loc != nil {
			return src[:loc[0]] + repl + src[loc[1]:], true
		}
	}
}

// leftPattern is the regexp for a replace's left side: a space outside a
// string or rune literal matches any whitespace run (how a one-line
// suggestion names a multi-line span); inside a literal it matches exactly
// one space. closed is false when the text ends inside a literal.
func leftPattern(left string) (pattern string, closed bool) {
	var b strings.Builder
	var quote rune
	escaped := false
	for _, r := range left {
		switch {
		case quote != 0:
			b.WriteString(regexp.QuoteMeta(string(r)))
			switch {
			case escaped:
				escaped = false
			case r == '\\' && quote != '`':
				escaped = true
			case r == quote:
				quote = 0
			}
		case r == ' ':
			b.WriteString(`\s+`)
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
			if r == '"' || r == '`' || r == '\'' {
				quote = r
			}
		}
	}
	return b.String(), quote == 0
}
