package evo

import (
	"io"
	"time"

	"github.com/zachbornheimer/evident-output/internal/engine"
)

func AlsoWrite(w io.Writer) Option               { return engine.AlsoWrite(w) }
func Clock(ts TimeSource) Option                 { return engine.Clock(ts) }
func DataProjection() Option                     { return engine.DataProjection() }
func DebugAddSource() Option                     { return engine.DebugAddSource() }
func DebugHistory() Option                       { return engine.DebugHistory() }
func DebugLevel(level LogLevel) Option           { return engine.DebugLevel(level) }
func DebugPane(opts ...DebugPaneOption) Option   { return engine.DebugPane(opts...) }
func Diagnostics(w io.Writer) Option             { return engine.Diagnostics(w) }
func DryRun() Option                             { return engine.DryRun() }
func ExternalProjection() Option                 { return engine.ExternalProjection() }
func MaxEntities(n int) Option                   { return engine.MaxEntities(n) }
func MaxEvents(n int) Option                     { return engine.MaxEvents(n) }
func MaxFrameRate(framesPerSecond int) Option    { return engine.MaxFrameRate(framesPerSecond) }
func NoColor() Option                            { return engine.NoColor() }
func Plain() Option                              { return engine.Plain() }
func Redact(r Redactor) Option                   { return engine.Redact(r) }
func ResultStream(w io.Writer) Option            { return engine.ResultStream(w) }
func Stdin(r io.Reader) Option                   { return engine.Stdin(r) }
func Strict() Option                             { return engine.Strict() }
func Terminal(driver TerminalDriver) Option      { return engine.Terminal(driver) }
func Title(subject string) Option                { return engine.Title(subject) }
func To(w io.Writer) Option                      { return engine.To(w) }
func VisibilityDelay(delay time.Duration) Option { return engine.VisibilityDelay(delay) }
func Width(columns int) Option                   { return engine.Width(columns) }
