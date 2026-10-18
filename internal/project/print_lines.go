package project

import (
	"strings"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

const (
	// maxPrintLine bounds one printed line, so a caller that never ends
	// one cannot grow the pending fragment without limit.
	maxPrintLine = 64 * 1024
	// printLineCut marks where a too-long line was cut.
	printLineCut = "…[line truncated]"
)

// PrintLines assembles printed fragments into lines. CR and CRLF end a line
// as LF does, a line longer than maxPrintLine is cut and marked, and a
// fragment printed at another visibility first ends the pending line, so a
// verbose line never borrows a normal one's text.
type PrintLines struct {
	pending    strings.Builder
	visibility record.Visibility
	// emit receives each complete line at the visibility it was printed.
	emit func(line string, visibility record.Visibility)
}

// NewPrintLines hands each complete line to emit.
func NewPrintLines(emit func(line string, visibility record.Visibility)) *PrintLines {
	return &PrintLines{emit: emit}
}

// Add prints s at visibility.
func (p *PrintLines) Add(s string, visibility record.Visibility) {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	if p.visibility != visibility {
		p.Flush()
	}
	p.visibility = visibility
	for len(s) > 0 {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			p.pending.WriteString(s)
			if p.pending.Len() > maxPrintLine {
				p.Flush()
			}
			return
		}
		p.pending.WriteString(s[:i])
		p.emitPending()
		s = s[i+1:]
	}
}

// Flush ends the pending line, if any.
func (p *PrintLines) Flush() {
	if p.pending.Len() > 0 {
		p.emitPending()
	}
}

func (p *PrintLines) emitPending() {
	line := TruncateUTF8(p.pending.String(), maxPrintLine, printLineCut)
	p.pending.Reset()
	p.emit(line, p.visibility)
}
