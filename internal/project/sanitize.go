package project

import "github.com/zachbornheimer/evident-output/v2/internal/record"

// Text and TruncateUTF8 forward to internal/record, which owns
// sanitization because a Problem or Fact is sanitized as it is stored.

// Text neutralizes control characters and invalid UTF-8 for a single-line field.
func Text(s string) string { return record.SanitizeText(s) }

// TruncateUTF8 trims s to at most max bytes without splitting a rune, then
// appends suffix when it truncated.
func TruncateUTF8(s string, max int, suffix string) string {
	return record.TruncateUTF8(s, max, suffix)
}
