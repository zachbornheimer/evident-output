package evo

import (
	"io"
	"log/slog"
	"time"

	"github.com/zachbornheimer/evident-output/internal/engine"
)

// Init is the sole Output constructor. It builds an Output from cfg,
// installs it as the package-level default, and arms first paint — call
// once, in main, before any I/O.
func Init(configs ...Config) *Output { return engine.Init(configs...) }

// SetDefault installs out as the package-level default Output.
func SetDefault(out *Output) { engine.SetDefault(out) }

// Default returns the package-level default Output, lazily creating one
// with a zero Config the first time it's needed.
func Default() *Output { return engine.Default() }

// Task declares (or, for a repeated name, returns) a Task on the default instance.
func Task(name string, args ...any) *TaskHandle { return engine.Task(name, args...) }

// Sequence declares (or, for a repeated name, returns) a self-managing,
// ordered task container on the default instance.
func Sequence(name string, args ...any) *SequenceHandle {
	return engine.Sequence(name, args...)
}

// Reason returns a get-or-create taxonomy Reason by name on the default instance.
func Reason(name string, args ...any) TaxonomyReason { return engine.Reason(name, args...) }

// Print formats like fmt.Sprint and enqueues human-facing text on the default instance.
func Print(args ...any) { engine.Print(args...) }

// Printf formats like fmt.Sprintf and enqueues human-facing text on the default instance.
func Printf(format string, args ...any) { engine.Printf(format, args...) }

// Println formats like fmt.Sprintln and enqueues a complete line on the default instance.
func Println(args ...any) { engine.Println(args...) }

// Verbose returns a Printer scoped to Verbose visibility on the default instance.
func Verbose() *Printer { return engine.Verbose() }

// SlogHandler returns a slog.Handler journaling to the default instance.
func SlogHandler() slog.Handler { return engine.SlogHandler() }

// Run executes run, finishes the default Output, and returns the conclusion exit code.
func Run(run func() error) int { return engine.Run(run) }

// Main executes run and os.Exit's with the conclusion code.
func Main(run func() error) { engine.Main(run) }

// MainWith executes run against out and os.Exit's with the conclusion code.
func MainWith(out *Output, run func(*Output) error) { engine.MainWith(out, run) }

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
func Warn(summary string, args ...any)     { engine.Warn(summary, args...) }
func Delay(d time.Duration) *time.Duration { return engine.Delay(d) }
func DefaultConfig() Config                { return engine.DefaultConfig() }
func IsCharDevice(w io.Writer) bool        { return engine.IsCharDevice(w) }
func Pluralize(quantity int64, singular string) string {
	return engine.Pluralize(quantity, singular)
}
func RenderPlain(s Snapshot, opts PlainOptions) ([]byte, error) {
	return engine.RenderPlain(s, opts)
}
func TruncateNames(names []string, visible int, profile ...GlyphProfile) string {
	return engine.TruncateNames(names, visible, profile...)
}

func KeepLastLines(n int) EvidenceOption    { return engine.KeepLastLines(n) }
func MaxEvidenceBytes(n int) EvidenceOption { return engine.MaxEvidenceBytes(n) }
func MirrorToDebug() EvidenceOption         { return engine.MirrorToDebug() }
func MirrorToDiagnostics() EvidenceOption   { return engine.MirrorToDiagnostics() }

func NewestFirst() DebugPaneOption         { return engine.NewestFirst() }
func OldestFirst() DebugPaneOption         { return engine.OldestFirst() }
func PaneHeight(lines int) DebugPaneOption { return engine.PaneHeight(lines) }
func PreserveDebugTail() DebugPaneOption   { return engine.PreserveDebugTail() }

func Affected(n int) EffectOption         { return engine.Affected(n) }
func ID(id string) EntityOption           { return engine.ID(id) }
func StartPhase(text string) EntityOption { return engine.StartPhase(text) }
func ForSkip() ReasonOption               { return engine.ForSkip() }
func OnTask(taskName string) ReasonOption { return engine.OnTask(taskName) }
