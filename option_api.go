package evo

import (
	"io"
	"time"

	"github.com/zachbornheimer/evident-output/internal/engine"
)

func alsoWrite(w io.Writer) Option               { return engine.AlsoWrite(w) }
func clock(ts TimeSource) Option                 { return engine.Clock(ts) }
func dataProjection() Option                     { return engine.DataProjection() }
func debugAddSource() Option                     { return engine.DebugAddSource() }
func debugHistory() Option                       { return engine.DebugHistory() }
func debugLevel(level LogLevel) Option           { return engine.DebugLevel(level) }
func debugPane(opts ...DebugPaneOption) Option   { return engine.DebugPane(opts...) }
func diagnostics(w io.Writer) Option             { return engine.Diagnostics(w) }
func dryRun() Option                             { return engine.DryRun() }
func externalProjection() Option                 { return engine.ExternalProjection() }
func maxEntities(n int) Option                   { return engine.MaxEntities(n) }
func maxEvents(n int) Option                     { return engine.MaxEvents(n) }
func maxFrameRate(framesPerSecond int) Option    { return engine.MaxFrameRate(framesPerSecond) }
func noColor() Option                            { return engine.NoColor() }
func plain() Option                              { return engine.Plain() }
func redact(r Redactor) Option                   { return engine.Redact(r) }
func resultStream(w io.Writer) Option            { return engine.ResultStream(w) }
func stdin(r io.Reader) Option                   { return engine.Stdin(r) }
func strict() Option                             { return engine.Strict() }
func terminal(driver TerminalDriver) Option      { return engine.Terminal(driver) }
func title(subject string) Option                { return engine.Title(subject) }
func to(w io.Writer) Option                      { return engine.To(w) }
func visibilityDelay(delay time.Duration) Option { return engine.VisibilityDelay(delay) }
func width(columns int) Option                   { return engine.Width(columns) }
