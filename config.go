package evo

import "io"

// Config is advanced construction for tests and embedding.
type Config struct {
	Subject      string
	Primary      io.Writer
	Diagnostic   io.Writer
	Projection   ProjectionPolicy
	Clock        TimeSource
	Redactor     Redactor
	Capabilities *CapabilityProfile
	Strict       bool
	Plain        bool
	NoColor      bool
	Width        int
}

// NewWithConfig builds an Output from advanced configuration.
func NewWithConfig(cfg Config) (*Output, error) {
	var opts []Option
	if cfg.Primary != nil {
		opts = append(opts, To(cfg.Primary))
	}
	if cfg.Diagnostic != nil {
		opts = append(opts, Diagnostics(cfg.Diagnostic))
	}
	switch cfg.Projection {
	case ProjectionData:
		opts = append(opts, DataProjection())
	case ProjectionExternal:
		opts = append(opts, ExternalProjection())
	}
	if cfg.Clock != nil {
		opts = append(opts, Clock(cfg.Clock))
	}
	if cfg.Redactor != nil {
		opts = append(opts, Redact(cfg.Redactor))
	}
	if cfg.Strict {
		opts = append(opts, Strict())
	}
	if cfg.Plain {
		opts = append(opts, Plain())
	}
	if cfg.NoColor {
		opts = append(opts, NoColor())
	}
	if cfg.Width > 0 {
		opts = append(opts, Width(cfg.Width))
	}
	if cfg.Subject != "" {
		return For(cfg.Subject, opts...), nil
	}
	return New(opts...), nil
}
