package project

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// detailColumn is the display column (in runes) at which want starts in the
// one line of out.
func detailColumn(t *testing.T, out, want string) int {
	t.Helper()
	line := strings.TrimRight(out, "\n")
	i := strings.Index(line, want)
	if i < 0 {
		t.Fatalf("row %q does not contain %q", line, want)
	}
	return utf8.RuneCountInString(line[:i])
}

// TestInlineFactStartsInSummaryColumn: an inline Fact is detail text like a
// Summary, so both start in one column when the rows share a name column.
// The Fact is not padded to line up with an inline "! text" no 2.0 row draws.
func TestInlineFactStartsInSummaryColumn(t *testing.T) {
	t.Parallel()
	style := Style{Profile: GlyphsUnicode, Verbose: true}
	const nameWidth = len("working tree")

	var withSummary, withFact strings.Builder
	WriteTaskAligned(&withSummary, record.TaskSnapshot{
		Name: "branches", State: record.Succeeded, Summary: "2 branches need attention",
	}, nameWidth, style)
	WriteTaskAligned(&withFact, record.TaskSnapshot{
		Name: "working tree", State: record.Succeeded,
		Facts: []record.Fact{{Name: "HEAD", Value: "main"}},
	}, nameWidth, style)

	summaryAt := detailColumn(t, withSummary.String(), "2 branches")
	factAt := detailColumn(t, withFact.String(), "HEAD  main")
	if factAt != summaryAt {
		t.Fatalf("inline Fact starts at column %d, Summary at %d:\n%s%s", factAt, summaryAt, withSummary.String(), withFact.String())
	}
}
