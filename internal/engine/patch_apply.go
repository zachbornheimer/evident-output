package engine

import (
	"fmt"
	"strings"
)

func applyHunks(src []byte, hunks []patchHunk) ([]byte, error) {
	lines, trailingNL := splitFileLines(src)
	out := make([]string, 0, len(lines))
	pos := 0
	for _, h := range hunks {
		target, err := hunkSourceIndex(h, len(lines))
		if err != nil {
			return nil, err
		}
		if target < pos {
			return nil, fmt.Errorf("%w: overlapping hunks", ErrPatchMalformed)
		}
		out = append(out, lines[pos:target]...)
		pos = target
		for _, hl := range h.lines {
			switch hl.kind {
			case ' ', '-':
				if pos >= len(lines) || lines[pos] != hl.text {
					return nil, fmt.Errorf("%w: hunk does not match source", ErrPatchMalformed)
				}
				if hl.kind == ' ' {
					out = append(out, lines[pos])
				}
				pos++
			case '+':
				out = append(out, hl.text)
			}
		}
		if last := hunkLastLine(h); last != nil && last.noNL {
			trailingNL = false
		} else if pos == len(lines) {
			trailingNL = sourceTrailingNL(h, trailingNL)
		}
	}
	if pos > len(lines) {
		return nil, fmt.Errorf("%w: hunk overruns source", ErrPatchMalformed)
	}
	out = append(out, lines[pos:]...)
	return joinFileLines(out, trailingNL), nil
}

func hunkSourceIndex(h patchHunk, nlines int) (int, error) {
	if h.oldCount == 0 {
		if h.oldStart < 0 || h.oldStart > nlines {
			return 0, fmt.Errorf("%w: hunk start out of range", ErrPatchMalformed)
		}
		return h.oldStart, nil
	}
	idx := h.oldStart - 1
	if idx < 0 || idx > nlines {
		return 0, fmt.Errorf("%w: hunk start out of range", ErrPatchMalformed)
	}
	return idx, nil
}

func hunkLastLine(h patchHunk) *patchLine {
	if len(h.lines) == 0 {
		return nil
	}
	return &h.lines[len(h.lines)-1]
}

func sourceTrailingNL(h patchHunk, current bool) bool {
	last := hunkLastLine(h)
	if last == nil {
		return current
	}
	if last.kind == '+' || last.kind == ' ' {
		return !last.noNL
	}
	return current
}

func splitFileLines(src []byte) (lines []string, trailingNL bool) {
	if len(src) == 0 {
		return nil, false
	}
	trailingNL = src[len(src)-1] == '\n'
	s := string(src)
	if trailingNL {
		s = s[:len(s)-1]
	}
	return strings.Split(s, "\n"), trailingNL
}

func joinFileLines(lines []string, trailingNL bool) []byte {
	if len(lines) == 0 {
		if trailingNL {
			return []byte("\n")
		}
		return []byte{}
	}
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
	}
	if trailingNL {
		b.WriteByte('\n')
	}
	return []byte(b.String())
}
