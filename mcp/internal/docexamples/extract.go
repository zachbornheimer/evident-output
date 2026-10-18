// Package docexamples extracts the ```go code fences from the library's
// public docs and drift-checks them against compiled fixtures under
// fixtures/, so a doc or MCP-served snippet cannot silently drift into
// syntax that no longer matches the shipped API.
//
// The fixtures are ordinary buildable Go source. `go build ./...` compiles
// them like any other package in this module — a broken snippet fails the
// build with a real compiler error at the fixture's file/line, not a lint
// guess. TestDocFencesMatchFixtures (sync_test.go) is the other half: it
// proves each fixture's marked snippet region is byte-identical to the
// fence it claims to cover, so editing the doc without updating the
// fixture (or vice versa) fails loudly instead of silently drifting apart.
package docexamples

import (
	"bufio"
	"bytes"
	"strings"
)

// GoFence is one ```go ... ``` fenced code block extracted from a Markdown
// document, in document order.
type GoFence struct {
	// Content is the fenced block's body exactly as written, excluding the
	// fence lines themselves, with a trailing newline after the last line.
	Content string
	// Line is the 1-based line number of the opening ```go fence line.
	Line int
}

// ExtractGoFences returns every fenced code block in md whose info string is
// exactly "go" — "gotemplate", "go-ish", and unlabelled ``` fences never
// match, so only blocks explicitly marked as Go source are extracted.
func ExtractGoFences(md []byte) []GoFence {
	var fences []GoFence

	scanner := bufio.NewScanner(bytes.NewReader(md))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	lineNo := 0
	inFence := false
	fenceStartLine := 0
	var body strings.Builder
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		switch {
		case !inFence && trimmed == "```go":
			inFence = true
			fenceStartLine = lineNo
			body.Reset()
		case inFence && trimmed == "```":
			inFence = false
			fences = append(fences, GoFence{Content: body.String(), Line: fenceStartLine})
		case inFence:
			body.WriteString(line)
			body.WriteByte('\n')
		}
	}

	return fences
}
