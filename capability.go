package evo

// colorLevel describes terminal color support. Unexported (C8): no public
// entry point ever returned this to a caller except detectCapabilities,
// which is unexported alongside it.
type colorLevel int

const (
	colorNone colorLevel = iota
	colorBasic
	color256
	colorTrue
)

// capabilityProfile holds terminal capability facts (§22). Unexported (C8).
type capabilityProfile struct {
	Interactive bool
	Color       colorLevel
	Unicode     bool
	Width       int
	Height      int
	NoColor     bool
}

// detectCapabilities builds a profile from options and environment-like
// hints. Unexported (C8: no external caller). It does not read the real
// environment in the core package without injection; callers pass
// NoColor/Width/Plain options instead.
func detectCapabilities(opts ...Option) capabilityProfile {
	cfg := config{
		width:      defaultWidth,
		debugLevel: LevelInfo,
	}
	for _, o := range opts {
		if o != nil {
			o.apply(&cfg)
		}
	}
	p := capabilityProfile{
		Interactive: !cfg.plain,
		Unicode:     true,
		Width:       cfg.width,
		Height:      24,
		NoColor:     cfg.noColor,
		Color:       colorBasic,
	}
	// Plain/non-interactive still allow semantic color on final reports unless NoColor.
	if cfg.noColor {
		p.Color = colorNone
	}
	if cfg.terminal != nil {
		if ls := asLive(cfg.terminal); ls != nil {
			p.Width = ls.Columns()
			p.Height = ls.Rows()
			p.Interactive = ls.IsInteractive() && p.Interactive
		}
	}
	return p
}
