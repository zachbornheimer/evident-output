package engine

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

// redactString applies the configured redactor.
//
// It takes no lock: the redactor is an Option, fixed when the Output is
// constructed and never reassigned, and the constructor's return is what
// publishes it. Reading it under Output.mu deadlocked the one path that
// needs it most — a scheduled failure auto-attaching its evidence tail,
// where resolve already holds that lock — so the lock bought nothing and
// cost the whole run.
func (o *Output) redactString(s string) string {
	if o == nil {
		return s
	}
	r := o.cfg.redactor
	if r == nil {
		return s
	}
	return r.RedactString(s)
}
