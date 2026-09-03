package evo

import (
	"fmt"
	"sync"
)

// defaultMu guards defaultOut, the package-level default instance installed
// by Init/SetDefault and read by Default and the package-level Task/Item/
// Print* facades below (mirrors slog.SetDefault).
var (
	defaultMu  sync.RWMutex
	defaultOut *Output
)

// Init builds an Output from cfg, installs it as the package-level default,
// and arms first paint — call once, in main, before any I/O.
//
//	func main() {
//	    evo.Init(evo.Config{Title: "repo-retire"})
//	    os.Exit(evo.Main(run))
//	}
func Init(cfg Config) *Output {
	out := New(cfg)
	SetDefault(out)
	out.arm()
	return out
}

// SetDefault installs out as the package-level default Output.
func SetDefault(out *Output) {
	defaultMu.Lock()
	defaultOut = out
	defaultMu.Unlock()
}

// Default returns the package-level default Output, lazily creating one with
// a zero Config the first time it's needed — package-level Task/Item/Print*
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
		defaultOut = New(Config{})
	}
	return defaultOut
}

// Task declares (or, for a repeated name, returns) a Task on the default
// instance. Calling Task with the same name twice returns the same handle —
// the identity a caller doing evo.Task("branches") from two call sites
// expects. name is a printf format when args are present (fmt.Sprintf
// semantics); the get-or-create key is the formatted name.
func Task(name string, args ...any) *TaskHandle {
	formatted, opts := formatEntityName(name, args)
	return Default().taskGetOrCreate(formatted, opts...)
}

// Item declares an Item on the default instance.
func Item(name string, opts ...EntityOption) *ItemHandle {
	args := make([]any, len(opts))
	for i, opt := range opts {
		args[i] = opt
	}
	return Default().Item(name, args...)
}

// Group declares (or, for a repeated name, returns) a self-managing task
// group on the default instance — see Group for the auto-lifecycle contract.
// name is a printf format when args are present (fmt.Sprintf semantics).
func Group(name string, args ...any) *GroupHandle {
	return Default().Group(name, args...)
}

// Reason returns a get-or-create taxonomy Reason by name on the default
// instance registry — duplicate strings merge into one bucket, so an inline
// evo.Reason("protected") at every call site is always legal; lifting it to a
// package var (var reasonProtected = evo.Reason("protected")) is optional,
// not required for correctness.
func Reason(name string, opts ...ReasonOption) TaxonomyReason {
	return Default().reasonGetOrCreate(name, opts...)
}

// Reasonf returns a get-or-create taxonomy Reason on the default instance
// using a printf-formatted name (fmt.Sprintf semantics) — the formatted text
// is the get-or-create key, same identity rule as Reason.
func Reasonf(format string, args ...any) TaxonomyReason {
	return Reason(fmt.Sprintf(format, args...))
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
	return Default().Verbose()
}
