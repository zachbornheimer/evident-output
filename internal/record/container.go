package record

// Container is the truth of one Group or Sequence: what it said about itself.
// Its verdict is not stored here; it derives from its members. It lives
// inside a Run and shares the Run's mutex, like a Task.
type Container struct {
	run     *Run
	summary string
}

// NewContainer starts the record of a Group or Sequence in this run.
func (r *Run) NewContainer() *Container { return &Container{run: r} }

// Summary is the one line of result text for the container's row.
func (c *Container) Summary() string {
	c.run.mu.Lock()
	defer c.run.mu.Unlock()
	return c.summary
}

// SetSummary replaces the result text with a sanitized text; empty clears it.
func (c *Container) SetSummary(text string) {
	c.run.mu.Lock()
	defer c.run.mu.Unlock()
	c.summary = SanitizeText(text)
}
