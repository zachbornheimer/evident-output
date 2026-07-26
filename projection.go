package evo

// ProjectionPolicy selects how output is emitted.
type ProjectionPolicy int

const (
	// ProjectionAuto chooses based on options/TTY hints.
	ProjectionAuto ProjectionPolicy = iota
	// ProjectionHuman is interactive or plain human output.
	ProjectionHuman
	// ProjectionData keeps machine data on the primary writer; UI on diagnostic.
	ProjectionData
	// ProjectionExternal disables inline rendering; snapshots only.
	ProjectionExternal
)

// DataProjection selects data-command mode (UI/progress on diagnostic stream).
func DataProjection() Option {
	return optionFunc(func(c *config) { c.projection = ProjectionData })
}

// ExternalProjection selects snapshot-only host rendering.
func ExternalProjection() Option {
	return optionFunc(func(c *config) { c.projection = ProjectionExternal; c.nonInteractive = true })
}
