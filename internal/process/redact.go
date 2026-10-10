package process

// Redactor redacts sensitive values before journal, Capture retention, and
// human rendering.
type Redactor interface {
	// RedactString returns a display-safe form of s.
	RedactString(s string) string
}

// NoopRedactor leaves strings unchanged.
type NoopRedactor struct{}

// RedactString implements Redactor.
func (NoopRedactor) RedactString(s string) string { return s }
