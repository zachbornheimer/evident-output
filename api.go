package evo

import (
	"context"
	"io"
	"log/slog"
	"time"

	"github.com/zachbornheimer/evident-output/internal/engine"
)

// RunFunc is the shape of application work handed to Run/Main/Output.Run —
// a context.Context carries cancellation (wired to SIGINT/SIGTERM by those
// entrypoints) in place of the pre-v0.6 no-context func() error form.
type RunFunc = engine.RunFunc

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

// Task declares a Task on the default instance.
func Task(name string) *TaskHandle { return wrapTask(engine.Task(name)) }

// Sequence declares a self-managing, ordered task container on the default
// instance.
func Sequence(name string) *SequenceHandle {
	return wrapSequence(engine.Sequence(name))
}

func Group(name string) *GroupHandle { return wrapGroup(engine.Group(name)) }

// Reason returns a get-or-create taxonomy Reason by name on the default instance.
func Reason(name string) TaxonomyReason { return TaxonomyReason{inner: engine.Reason(name)} }

// Print formats like fmt.Sprint and enqueues human-facing text on the default instance.
func Print(args ...any) { engine.Print(args...) }

// Printf formats like fmt.Sprintf and enqueues human-facing text on the default instance.
func Printf(format string, args ...any) { engine.Printf(format, args...) }

// Println formats like fmt.Sprintln and enqueues a complete line on the default instance.
func Println(args ...any) { engine.Println(args...) }

// Verbose returns a Printer scoped to Verbose visibility on the default instance.
func Verbose() *Printer { return wrapPrinter(engine.Verbose()) }

// SlogHandler returns a slog.Handler journaling to the default instance.
func SlogHandler() slog.Handler { return engine.SlogHandler() }

// Run executes run against the default Output and returns the Result
// (Conclusion plus the application error, if any); it never exits the
// process.
func Run(ctx context.Context, run RunFunc) Result { return engine.Run(ctx, run) }

// Main executes run against the default Output and returns the derived exit
// code; it does not itself call os.Exit — callers write
// os.Exit(evo.Main(run)).
func Main(run RunFunc) int { return engine.Main(run) }

// Confirm asks question on the default instance and returns whether the user accepted.
func Confirm(question string, opts ...ConfirmOption) bool {
	return engine.Confirm(question, opts...)
}

func AssumeYes(v bool) ConfirmOption              { return engine.AssumeYes(v) }
func ConfirmDetail(lines ...string) ConfirmOption { return engine.ConfirmDetail(lines...) }
func Destructive() ConfirmOption                  { return engine.Destructive() }
func PolicyFlag(flag string) ConfirmOption        { return engine.PolicyFlag(flag) }
func PolicyHint(command string, args ...string) ConfirmOption {
	return engine.PolicyHint(command, args...)
}

func Fact(name, value string)              { engine.Fact(name, value) }
func Warn(summary string)                  { engine.Warn(summary) }
func Delay(d time.Duration) *time.Duration { return engine.Delay(d) }
func DefaultConfig() Config                { return engine.DefaultConfig() }
func IsCharDevice(w io.Writer) bool        { return engine.IsCharDevice(w) }
func Pluralize(quantity int64, singular string) string {
	return engine.Pluralize(quantity, singular)
}
func RenderPlain(s Snapshot, opts PlainOptions) ([]byte, error) {
	return engine.RenderPlain(s, opts)
}
func TruncateNames(names []string, visible int) string {
	return engine.TruncateNames(names, visible)
}

func KeepLastLines(n int) EvidenceOption    { return engine.KeepLastLines(n) }
func MaxEvidenceBytes(n int) EvidenceOption { return engine.MaxEvidenceBytes(n) }
func MirrorToDebug() EvidenceOption         { return engine.MirrorToDebug() }
func MirrorToDiagnostics() EvidenceOption   { return engine.MirrorToDiagnostics() }

func NewestFirst() DebugPaneOption         { return engine.NewestFirst() }
func OldestFirst() DebugPaneOption         { return engine.OldestFirst() }
func PaneHeight(lines int) DebugPaneOption { return engine.PaneHeight(lines) }
func PreserveDebugTail() DebugPaneOption   { return engine.PreserveDebugTail() }

func Affected(n int) MutationOption { return engine.Affected(n) }

// ID sets a stable machine key. Superseded: Task is name-only.
func ID(id string) EntityOption { return engine.ID(id) }

// StartPhase sets a task's first doing-text at declare time. Superseded: call Doing.
func StartPhase(text string) EntityOption { return engine.StartPhase(text) }
func ForSkip() ReasonOption               { return engine.ForSkip() }
func OnTask(taskName string) ReasonOption { return engine.OnTask(taskName) }
