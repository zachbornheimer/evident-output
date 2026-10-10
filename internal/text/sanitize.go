package text

import "github.com/zachbornheimer/evident-output/internal/record"

// Text, Block and TruncateUTF8 forward to internal/record, which owns
// sanitization because a Problem or Fact is sanitized as it is stored. The
// names stay so engine and render need no edit yet; slice 6 repoints their
// callers at record and deletes this file.

// Text neutralizes control characters and invalid UTF-8 for a single-line field.
func Text(s string) string { return record.SanitizeText(s) }

// Block neutralizes control characters but keeps newlines, for multi-line fields.
func Block(s string) string { return record.SanitizeBlock(s) }

// TruncateUTF8 trims s to at most max bytes without splitting a rune, then
// appends suffix when it truncated.
func TruncateUTF8(s string, max int, suffix string) string {
	return record.TruncateUTF8(s, max, suffix)
}
