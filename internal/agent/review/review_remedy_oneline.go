package review

import (
	"go/scanner"
	"go/token"
	"strings"
)

// A Finding.Suggestion is one line (review.go). An edit that spans statements
// is therefore written as `replace <left> with <right>` where every newline run
// in <left> is a single space, which matches any whitespace run in the source,
// and <right> is one line, statements separated by "; ".
//
// The renderer works on Go tokens, never on raw text, so a string literal is
// copied byte for byte. It declines (ok=false) when one line cannot say the
// same thing: a comment would swallow the rest of the line, and a raw string
// that spans lines has a newline that is part of its value.

// oneLine renders src as one line. tidy also removes the padding inside
// parentheses and a trailing comma before ")", which a left side must keep
// because it has to match the source.
func oneLine(src string, tidy bool) (string, bool) {
	file := token.NewFileSet().AddFile("", -1, len(src))
	var s scanner.Scanner
	var bad bool
	s.Init(file, []byte(src), func(token.Position, string) { bad = true }, scanner.ScanComments)

	type piece struct {
		tok  token.Token
		text string
		gap  string
	}
	var pieces []piece
	prevEnd := 0
	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}
		if tok == token.SEMICOLON && lit == "\n" {
			continue // the scanner's automatic semicolon is a newline, not text
		}
		start := file.Offset(pos)
		text := lit
		if text == "" {
			text = tok.String()
		}
		if tok == token.COMMENT || (tok == token.STRING && strings.Contains(text, "\n")) {
			return "", false
		}
		gap := src[prevEnd:start]
		if len(pieces) == 0 {
			gap = ""
		} else if strings.Contains(gap, "\n") {
			gap = " "
		}
		pieces = append(pieces, piece{tok, text, gap})
		prevEnd = start + len(text)
	}
	if bad {
		return "", false
	}

	var out strings.Builder
	for i, p := range pieces {
		gap := p.gap
		if tidy && i > 0 {
			if pieces[i-1].tok == token.LPAREN || p.tok == token.RPAREN {
				gap = ""
			}
			if p.tok == token.RPAREN && pieces[i-1].tok == token.COMMA {
				trimmed := strings.TrimSuffix(out.String(), ",")
				out.Reset()
				out.WriteString(trimmed)
			}
		}
		out.WriteString(gap)
		out.WriteString(p.text)
	}
	return out.String(), true
}

// replaceSuggestion renders one single-line replace edit, or reports that the
// edit cannot be written on one line without altering the code.
func replaceSuggestion(left, right string) (string, bool) {
	l, ok := oneLine(left, false)
	if !ok {
		return "", false
	}
	r, ok := oneLine(right, true)
	if !ok {
		return "", false
	}
	return "replace " + l + " with " + r, true
}
