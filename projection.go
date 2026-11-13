package evo

// DataProjection selects data-command mode (UI/progress on diagnostic
// stream) — a self-documenting marker at the call site; the routing itself
// comes from pairing it with to(stderr)/withDiagnostics(stderr) (configToOptions).
func dataProjection() Option {
	return optionFunc(func(*config) {})
}

// ExternalProjection selects snapshot-only host rendering.
func externalProjection() Option {
	return optionFunc(func(c *config) { c.plain = true })
}
