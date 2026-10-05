package evo

import (
	"context"

	"github.com/zachbornheimer/evident-output/internal/engine"
)

// RunFunc is the shape of application work handed to Run/Main/Output.Run —
// a context.Context carries cancellation (wired to SIGINT/SIGTERM by those
// entrypoints) in place of the pre-v0.6 no-context func() error form.
type RunFunc = engine.RunFunc

type Config = engine.Config

// Init is the sole Output constructor. It builds an Output from cfg,
// installs it as the package-level default, and arms first paint — call
// once, in main, before any I/O.
func Init(configs ...Config) *Output { return wrapOutput(engine.Init(configs...)) }

// SetDefault installs out as the package-level default Output.
func SetDefault(out *Output) {
	if out == nil {
		engine.SetDefault(nil)
		return
	}
	engine.SetDefault(out.inner)
}

// Default returns the package-level default Output, lazily creating one
// with a zero Config the first time it's needed.
func Default() *Output { return wrapOutput(engine.Default()) }

// Run executes run against the default Output and returns the Result
// (Conclusion plus the application error, if any); it never exits the
// process.
func Run(ctx context.Context, run RunFunc) Result { return engine.Run(ctx, run) }

// Main executes run against the default Output and returns the derived exit
// code; it does not itself call os.Exit — callers write
// os.Exit(evo.Main(run)).
func Main(run RunFunc) int { return engine.Main(run) }

func DefaultConfig() Config { return engine.DefaultConfig() }
