package review_test

import (
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/internal/agent/review"
)

// A TerminalDriver is the renderer's sink: its own Write* methods writing
// to the writer it holds are how frames reach the terminal, not output
// competing with them. The same Fprintf from any other method still fires.
func TestStreamRulesExemptATerminalDriverWritingItsOwnSink(t *testing.T) {
	const src = `package main

import (
	"fmt"
	"io"

	evo "github.com/zachbornheimer/evident-output"
)

type frameLog struct{ w io.Writer }

func (f *frameLog) WriteLive(text string)    { _, _ = fmt.Fprintf(f.w, "frame\n%s\n", text) }
func (f *frameLog) WriteDurable(line string) { _, _ = fmt.Fprintln(f.w, line) }

type report struct{ w io.Writer }

func (r *report) Emit(line string) { _, _ = fmt.Fprintln(r.w, line) }

func main() { evo.Init(evo.Config{}) }
`
	res := review.GoSource("driver.go", src)
	var lines []int
	for _, f := range res.Findings {
		if f.RuleID == "STREAM-003" || f.RuleID == "EVO-LIVE-001" {
			lines = append(lines, f.Line)
		}
	}
	if len(lines) != 2 || lines[0] != 17 || lines[1] != 17 {
		t.Fatalf("STREAM-003/EVO-LIVE-001 lines = %v, want exactly [17 17] (report.Emit only, not the driver's WriteLive/WriteDurable)", lines)
	}
}
