package evo

import (
	"context"
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
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

// AfterRunFunc writes human text that belongs below the run's footer: w is
// the human stream, r the finished Result.
type AfterRunFunc = engine.AfterRunFunc

// AfterRun registers fn on the default Output to render after the footer,
// once Main/Run has finished. Text printed after the run without it is
// dropped and reported. Machine projections never receive it.
func AfterRun(fn AfterRunFunc) { engine.AfterRun(fn) }

// AfterRun registers fn to render after the footer of o's Run (see the
// package-level AfterRun).
func (o *Output) AfterRun(fn AfterRunFunc) {
	if o == nil || o.inner == nil {
		return
	}
	o.inner.AfterRun(fn)
}

func DefaultConfig() Config { return engine.DefaultConfig() }

// Output, TaskHandle, and the other presentation handles are wrappers, not
// aliases: engine test helpers must not appear in go doc or the rec surface.
type Output struct{ inner *engine.Output }

func wrapOutput(inner *engine.Output) *Output {
	return wrap(inner, func() *Output { return &Output{inner: inner} })
}

// facaded is an engine handle that keeps its own public wrapper.
type facaded interface {
	comparable
	Facade() *engine.FacadeSlot
}

// wrap returns inner's one public wrapper, creating it on first use, so a
// handle compares equal to itself however many calls hand it out. The
// wrapper lives in inner's own slot, so it never outlives inner.
func wrap[I facaded, W any](inner I, newWrapper func() *W) *W {
	var zero I
	if inner == zero {
		return nil
	}
	return inner.Facade().Wrapper(func() any { return newWrapper() }).(*W)
}

func (o *Output) impl() *engine.Output {
	if o == nil {
		return nil
	}
	return o.inner
}

func (o *Output) Close() error {
	if o == nil || o.inner == nil {
		return nil
	}
	return o.inner.Close()
}

func (o *Output) Conclusion() Conclusion {
	if o == nil || o.inner == nil {
		return Conclusion{}
	}
	return o.inner.Conclusion()
}

func (o *Output) Context() context.Context {
	if o == nil || o.inner == nil {
		return context.Background()
	}
	return o.inner.Context()
}

func (o *Output) Err() error {
	if o == nil || o.inner == nil {
		return nil
	}
	return o.inner.Err()
}

func (o *Output) Finish() error {
	if o == nil || o.inner == nil {
		return nil
	}
	return o.inner.Finish()
}

// Run executes run against o and returns the Result (Conclusion plus the
// application error, if any); it never exits the process.
func (o *Output) Run(ctx context.Context, run RunFunc) Result {
	if o == nil || o.inner == nil {
		return Result{Conclusion: Conclusion{State: StateFailed, ExitCode: ExitFailed}}
	}
	return o.inner.Run(ctx, run)
}

// Events returns a copy of this instance's durable event journal, for
// EncodeJSONL and other machine-projection call sites that need the raw
// event stream rather than a point-in-time Snapshot.
func (o *Output) Events() []Event {
	if o == nil || o.inner == nil {
		return nil
	}
	return o.inner.Events()
}

// Result is the outcome of Run/Main/Output.Run — the finished Conclusion
// plus the application error the run callback returned, if any. Run and
// Output.Run return it directly; Main derives its int exit code from it.
// See EVIDENT_OUTPUT_ARCHITECTURE spec §1.1, §32.2.
type Result = core.Result

func Delay(d time.Duration) *time.Duration { return engine.Delay(d) }
