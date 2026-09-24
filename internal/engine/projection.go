package engine

// DataProjection selects data-command mode (UI/progress on diagnostic
// stream) — a self-documenting marker at the call site; the routing itself
// comes from pairing it with to(stderr)/withDiagnostics(stderr) (configToOptions).
func dataProjection() Option {
	return optionFunc(func(*config) {})
}

// ExternalProjection selects snapshot-only host rendering. It chooses how
// the run renders, not who owns its lifecycle: that is Config.Embedded
// (DEC-CANCEL-005).
func externalProjection() Option {
	return optionFunc(func(c *config) { c.plain, c.external = true, true })
}
