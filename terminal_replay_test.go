package evo

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// terminalControl matches the escape sequences the live driver writes: cursor up, line erase, and the cursor
// show and hide private modes.
var terminalControl = regexp.MustCompile(`^\x1b\[(\?25[hl]|\d*[AK])`)

// replayTerminal returns the rows a terminal shows after receiving raw, with no scrollback limit. It
// understands carriage return, line feed, cursor up (CSI n A), erase line (CSI 2 K) and ignores cursor
// visibility, which is every control the live driver writes.
func replayTerminal(raw []byte) []string {
	rows := [][]rune{nil}
	row, col := 0, 0
	text := string(raw)
	for i := 0; i < len(text); {
		if text[i] == 0x1b {
			seq := terminalControl.FindString(text[i:])
			if seq == "" {
				i++
				continue
			}
			i += len(seq)
			switch seq[len(seq)-1] {
			case 'A':
				row = max(row-controlCount(seq), 0)
			case 'K':
				rows[row] = nil
			}
			continue
		}
		r, size := utf8.DecodeRuneInString(text[i:])
		i += size
		switch r {
		case '\r':
			col = 0
		case '\n':
			row++
			if row == len(rows) {
				rows = append(rows, nil)
			}
		default:
			rows[row] = placeRune(rows[row], col, r)
			col++
		}
	}
	screen := make([]string, len(rows))
	for i, r := range rows {
		screen[i] = strings.TrimRight(string(r), " ")
	}
	return screen
}

// controlCount is the numeric argument of a CSI sequence, one when absent.
func controlCount(seq string) int {
	digits := strings.TrimFunc(seq, func(r rune) bool { return r < '0' || r > '9' })
	n, err := strconv.Atoi(digits)
	if err != nil {
		return 1
	}
	return n
}

// placeRune writes r at col, padding the row with spaces up to it.
func placeRune(row []rune, col int, r rune) []rune {
	for len(row) < col {
		row = append(row, ' ')
	}
	if col == len(row) {
		return append(row, r)
	}
	row[col] = r
	return row
}
