package evo

import (
	"log/slog"
	"sync"
)

// defaultMu guards defaultOut, the package-level default instance installed
// by Init/SetDefault and read by Default and the package-level Task/Item/
// Print* facades below (mirrors slog.SetDefault).
var (
	defaultMu  sync.RWMutex
	defaultOut *Output
)

// Init is the sole Output constructor. It builds an Output from cfg,
// installs it as the package-level default, and arms first paint — call
// once, in main, before any I/O:
//
//	func main() {
//	    evo.Init(evo.Config{Title: "repo-retire"})
//	    evo.Main(run)
//	}
//
// evo.Init(evo.Config{}) (or evo.Init(evo.DefaultConfig())) builds an
// ordinary default instance. Config.Isolated returns an independent
// instance that never touches package state.
//
// Init fills still-zero Config fields from EVO_OUTPUT, EVO_COLOR,
// EVO_VERBOSE, EVO_DEBUG, and NO_COLOR before TTY inference. Explicit
// Config values win over env; env wins over TTY.
func Init(cfg Config) *Output {
	cfg = applyEnv(cfg)
	resolved := resolveConfig(cfg)
	out := newFromConfig(resolved)
	if !cfg.Isolated {
		SetDefault(out)
		out.arm()
	}
	if cfg.Subject != "" && !cfg.DryRun && !cfg.Preview {
		out.Println(cfg.Subject)
	}
	return out
}

// SetDefault installs out as the package-level default Output.
func SetDefault(out *Output) {
	defaultMu.Lock()
	defaultOut = out
	defaultMu.Unlock()
}

// Default returns the package-level default Output, lazily creating one with
// a zero Config the first time it's needed — package-level Task/Print*
// never panic even when the caller skipped Init.
func Default() *Output {
	defaultMu.RLock()
	out := defaultOut
	defaultMu.RUnlock()
	if out != nil {
		return out
	}

	defaultMu.Lock()
	defer defaultMu.Unlock()
	if defaultOut == nil {
		defaultOut = newFromConfig(resolveConfig(applyEnv(Config{})))
	}
	return defaultOut
}

// Task declares (or, for a repeated name, returns) a Task on the default
// instance. Calling Task with the same name twice returns the same handle —
// the identity a caller doing evo.Task("branches") from two call sites
// expects. name is a printf format when args are present (fmt.Sprintf
// semantics); the get-or-create key is the formatted name.
func Task(name string) *TaskHandle {
	return Default().taskGetOrCreate(name)
}

// Sequence declares (or, for a repeated name, returns) an ordered task
// container on the default instance — see Output.Sequence.
func Sequence(name string) *SequenceHandle {
	return Default().Sequence(name)
}

// Group declares (or, for a repeated name, returns) an independent
// collection of child tasks on the default instance — see Output.Group.
func Group(name string) *GroupHandle {
	return Default().Group(name)
}

// Reason returns a get-or-create taxonomy Reason by name on the default
// instance registry — duplicate strings merge into one bucket, so an inline
// evo.Reason("protected") at every call site is always legal; lifting it to a
// package var (var reasonProtected = evo.Reason("protected")) is optional,
// not required for correctness.
func Reason(name string) TaxonomyReason {
	return Default().reasonGetOrCreate(name)
}

// Print formats like fmt.Sprint and enqueues human-facing text on the default instance.
func Print(args ...any) {
	Default().Print(args...)
}

// Printf formats like fmt.Sprintf and enqueues human-facing text on the default instance.
func Printf(format string, args ...any) {
	Default().Printf(format, args...)
}

// Println formats like fmt.Sprintln and enqueues a complete line on the default instance.
func Println(args ...any) {
	Default().Println(args...)
}

// Verbose returns a Printer scoped to Verbose visibility on the default instance.
func Verbose() *Printer {
	return Default().at(VisibilityVerbose)
}

// SlogHandler returns a slog.Handler journaling to the default instance —
// package-level sugar (release-gate round 8 finding 6) matching Task/Verbose:
// a caller using the default-instance facade throughout a run should never
// have to reach for a hosted *Output just for the slog bridge. See
// Output.SlogHandler for the level policy and full contract.
func slogHandler() slog.Handler {
	return Default().slogHandler()
}
