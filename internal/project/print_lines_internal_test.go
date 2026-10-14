package project

import (
	"slices"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/record"
)

type printed struct {
	line string
	vis  record.Visibility
}

func collectLines() (*PrintLines, *[]printed) {
	var got []printed
	return NewPrintLines(func(line string, vis record.Visibility) { got = append(got, printed{line, vis}) }), &got
}

func TestPrintLinesSplitsOnEveryLineEnding(t *testing.T) {
	p, got := collectLines()
	p.Add("a\nb\r\nc\rpart", record.VisibilityNormal)
	p.Add("ial\n", record.VisibilityNormal)
	want := []printed{{"a", 0}, {"b", 0}, {"c", 0}, {"partial", 0}}
	if !slices.Equal(*got, want) {
		t.Fatalf("lines = %v, want %v", *got, want)
	}
}

func TestPrintLinesEndsAPendingLineWhenVisibilityChanges(t *testing.T) {
	p, got := collectLines()
	p.Add("normal", record.VisibilityNormal)
	p.Add("verbose\n", record.VisibilityVerbose)
	want := []printed{{"normal", record.VisibilityNormal}, {"verbose", record.VisibilityVerbose}}
	if !slices.Equal(*got, want) {
		t.Fatalf("lines = %v, want %v", *got, want)
	}
}

func TestPrintLinesCutsALineThatNeverEnds(t *testing.T) {
	p, got := collectLines()
	p.Add(strings.Repeat("x", maxPrintLine+1), record.VisibilityNormal)
	if len(*got) != 1 || !strings.HasSuffix((*got)[0].line, printLineCut) {
		t.Fatalf("lines = %d, want one cut line", len(*got))
	}
}
