package evo

// DataProjection selects data-command mode (UI/progress on diagnostic
// stream) — a self-documenting marker at the call site; the routing itself
// comes from pairing it with To(stderr)/Diagnostics(stderr) (configToOptions).
func DataProjection() Option {
	return optionFunc(func(*config) {})
}

// ExternalProjection selects snapshot-only host rendering.
func ExternalProjection() Option {
	return optionFunc(func(c *config) { c.plain = true })
}
