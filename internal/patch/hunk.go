package patch

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// hunkHeader matches "@@ -old[,count] +new[,count] @@"; an omitted count
// means 1.
var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// Hunk is one contiguous edit: the old and new line ranges it covers and
// the context, removed, and added lines between them.
type Hunk struct {
	OldStart, OldCount int
	NewStart, NewCount int
	lines              []hunkLine
}

// Hunk line operations.
const (
	opContext = ' '
	opRemove  = '-'
	opAdd     = '+'
	// opNoNewline marks the line before it as lacking a final newline.
	opNoNewline = '\\'
)

// hunkLine is one line of a hunk. eol is false only for a file's last line
// when the diff marks it "\ No newline at end of file".
type hunkLine struct {
	op   byte
	text string
	eol  bool
}

// bytes is the line as it appears in the file.
func (l hunkLine) bytes() []byte {
	if l.eol {
		return []byte(l.text + "\n")
	}
	return []byte(l.text)
}

// hunk reads one hunk: its header, then exactly as many old and new lines
// as the header counts.
func (p *parser) hunk() (Hunk, error) {
	header := trimCR(p.next())
	h, err := parseHunkHeader(header)
	if err != nil {
		return Hunk{}, err
	}
	oldLeft, newLeft := h.OldCount, h.NewCount
	for oldLeft > 0 || newLeft > 0 {
		if !p.more() {
			return Hunk{}, fmt.Errorf("%w: hunk %q is truncated", ErrMalformed, header)
		}
		raw := p.next()
		if raw == "" {
			// An editor stripped the lone space of an empty context line.
			raw = string(opContext)
		}
		switch raw[0] {
		case opContext:
			oldLeft, newLeft = oldLeft-1, newLeft-1
		case opRemove:
			oldLeft--
		case opAdd:
			newLeft--
		case opNoNewline:
			if err := h.markNoNewline(); err != nil {
				return Hunk{}, err
			}
			continue
		default:
			return Hunk{}, fmt.Errorf("%w: unexpected line %q in hunk %q", ErrMalformed, raw, header)
		}
		if oldLeft < 0 || newLeft < 0 {
			return Hunk{}, fmt.Errorf("%w: hunk %q has more lines than its header counts", ErrMalformed, header)
		}
		h.lines = append(h.lines, hunkLine{op: raw[0], text: raw[1:], eol: true})
	}
	if p.more() && strings.HasPrefix(p.peek(), string(opNoNewline)) {
		p.next()
		if err := h.markNoNewline(); err != nil {
			return Hunk{}, err
		}
	}
	return h, nil
}

func parseHunkHeader(header string) (Hunk, error) {
	m := hunkHeader.FindStringSubmatch(header)
	if m == nil {
		return Hunk{}, fmt.Errorf("%w: hunk header %q", ErrMalformed, header)
	}
	return Hunk{
		OldStart: atoiOr(m[1], 0), OldCount: atoiOr(m[2], 1),
		NewStart: atoiOr(m[3], 0), NewCount: atoiOr(m[4], 1),
	}, nil
}

// atoiOr parses a regexp-validated decimal, returning fallback when the
// optional group was absent.
func atoiOr(s string, fallback int) int {
	if s == "" {
		return fallback
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return n
}

func (h *Hunk) markNoNewline() error {
	if len(h.lines) == 0 {
		return fmt.Errorf("%w: no-newline marker before any hunk line", ErrMalformed)
	}
	h.lines[len(h.lines)-1].eol = false
	return nil
}

// Apply returns old with f's hunks applied. Each hunk must match at
// exactly the line its header names: a patch derives from one precise
// source, so a hunk that only matches elsewhere does not apply.
func (f File) Apply(old []byte) ([]byte, error) {
	src := fileLines(old)
	var out []byte
	pos := 0
	for i, h := range f.Hunks {
		start := h.OldStart - 1
		if h.OldCount == 0 {
			start = h.OldStart
		}
		if start < pos {
			return nil, fmt.Errorf("%w: %s hunk %d overlaps the previous hunk", ErrMalformed, f.Path, i+1)
		}
		if start > len(src) {
			return nil, fmt.Errorf("%w: %s hunk %d starts past the end of the file", ErrDoesNotApply, f.Path, i+1)
		}
		out = append(out, bytes.Join(src[pos:start], nil)...)
		pos = start
		for _, l := range h.lines {
			if l.op == opAdd {
				out = append(out, l.bytes()...)
				continue
			}
			if pos >= len(src) || !bytes.Equal(src[pos], l.bytes()) {
				return nil, fmt.Errorf("%w: %s hunk %d does not match line %d", ErrDoesNotApply, f.Path, i+1, pos+1)
			}
			if l.op == opContext {
				out = append(out, src[pos]...)
			}
			pos++
		}
	}
	out = append(out, bytes.Join(src[pos:], nil)...)
	return out, nil
}

// fileLines splits contents into lines that each keep their "\n"; only
// the last may lack one.
func fileLines(contents []byte) [][]byte {
	var lines [][]byte
	for len(contents) > 0 {
		end := bytes.IndexByte(contents, '\n') + 1
		if end == 0 {
			end = len(contents)
		}
		lines = append(lines, contents[:end])
		contents = contents[end:]
	}
	return lines
}
