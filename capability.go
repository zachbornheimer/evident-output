package evo

// ColorLevel describes terminal color support.
type ColorLevel int

const (
	ColorNone ColorLevel = iota
	ColorBasic
	Color256
	ColorTrue
)

// CapabilityProfile holds terminal capability facts (§22).
type CapabilityProfile struct {
	Interactive bool
	Color       ColorLevel
	Unicode     bool
	Width       int
	Height      int
	NoColor     bool
}

// DetectCapabilities builds a profile from options and environment-like hints.
// It does not read the real environment in the core package without injection;
// callers pass NoColor/Width/NonInteractive options instead.
func DetectCapabilities(opts ...Option) CapabilityProfile {
	cfg := config{
		width:      defaultWidth,
		debugLevel: LevelInfo,
	}
	for _, o := range opts {
		if o != nil {
			o.apply(&cfg)
		}
	}
	p := CapabilityProfile{
		Interactive: !cfg.nonInteractive && !cfg.plain,
		Unicode:     true,
		Width:       cfg.width,
		Height:      24,
		NoColor:     cfg.noColor,
		Color:       ColorBasic,
	}
	// Plain/non-interactive still allow semantic color on final reports unless NoColor.
	if cfg.noColor {
		p.Color = ColorNone
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
