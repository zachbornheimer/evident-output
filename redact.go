package evo

// Redactor redacts sensitive values before journal and human rendering.
type Redactor interface {
	// RedactString returns a display-safe form of s.
	RedactString(s string) string
}

// NoopRedactor leaves strings unchanged.
type NoopRedactor struct{}

// RedactString implements Redactor.
func (NoopRedactor) RedactString(s string) string { return s }

// Redact injects a redactor (applied to Debug/Line/problem detail paths).
func Redact(r Redactor) Option {
	return optionFunc(func(c *config) {
		if r != nil {
			c.redactor = r
		}
	})
}
