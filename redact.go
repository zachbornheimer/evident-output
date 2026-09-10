package evo

// Redactor redacts sensitive values before journal, Capture retention, and human rendering.
type Redactor interface {
	// RedactString returns a display-safe form of s.
	RedactString(s string) string
}

// noopRedactor leaves strings unchanged.
type noopRedactor struct{}

// RedactString implements Redactor.
func (noopRedactor) RedactString(s string) string { return s }

// Redact injects a redactor (Debug fields, Capture lines, problem detail paths).
func redact(r Redactor) Option {
	return optionFunc(func(c *config) {
		if r != nil {
			c.redactor = r
		}
	})
}

// redactString applies the configured redactor without holding capture locks long.
// Safe when called while Capture.mu is held (takes Output.mu briefly).
func (o *Output) redactString(s string) string {
	if o == nil {
		return s
	}
	o.mu.Lock()
	r := o.cfg.redactor
	o.mu.Unlock()
	if r == nil {
		return s
	}
	return r.RedactString(s)
}
