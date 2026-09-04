package evo

import (
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
//	    os.Exit(evo.Main(run))
//	}
//
// evo.Init() (zero args) or evo.Init(evo.Config{}) (or
// evo.Init(evo.DefaultConfig())) all build an ordinary default instance —
// Init is variadic (I9) so the zero-config call needs no empty Config{}
// literal. Passing more than one Config uses only the first; there is one
// construction call, not a merge.
//
// Config.Isolated returns an independent instance that skips both steps —
// it never touches package state (parallel tests, embedders holding their
// own *Output).
//
// Config.Options is the advanced raw-Option escape hatch for tests and
// specialized embedding; when set, ordinary Config fields (besides Title)
// are ignored, and — matching the retired NewWithOptions constructor this
// replaces — Init never installs the result as the package-level default or
// arms first paint, regardless of Isolated: a caller with direct Option
// control is expected to also control default-binding and arm() explicitly
// (e.g. via SetDefault, or Output.Run for the lifecycle MainWith used to own).
func Init(configs ...Config) *Output {
	cfg := resolveInitConfig(configs)
	if len(cfg.Options) > 0 {
		// Advanced/testing escape hatch: build directly from raw Options,
		// bypassing Config's ordinary stream/TTY/color inference entirely.
		// DryRun and Subject are additive and never conflict with a caller's
		// own Options, so they are still honored here instead of silently
		// dropped (I1) — everything else on Config is genuinely superseded
		// by the caller's explicit Option control.
		opts := cfg.Options
		if cfg.DryRun {
			opts = append(append([]Option{}, opts...), DryRun())
		}
		out := newOutput(cfg.Title, opts...)
		if cfg.Subject != "" {
			out.Println(cfg.Subject)
		}
		return out
	}
	out := newFromConfig(resolveConfig(cfg))
	if !cfg.Isolated {
		SetDefault(out)
		out.arm()
	}
	if cfg.Subject != "" {
		out.Println(cfg.Subject)
	}
	return out
}

// resolveInitConfig picks Init's effective Config from its variadic
// argument: zero args is the zero Config (evo.Init()), and one or more
// uses the first — there is one construction call, not a merge, so any
// argument past the first is ignored rather than erroring on a call shape
// no caller has a reason to make.
func resolveInitConfig(configs []Config) Config {
	if len(configs) == 0 {
		return Config{}
	}
	return configs[0]
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
		defaultOut = newFromConfig(resolveConfig(Config{}))
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
// not required for correctness. name is a printf format when args are
// present (fmt.Sprintf semantics) — one text spelling shared with
// Task/Group (C6); evo.ForSkip()/evo.OnTask(...) may be mixed into args in
// any position and still applies, exactly like Task's evo.ID.
func Reason(name string, args ...any) TaxonomyReason {
	formatted, opts := formatReasonName(name, args)
	return Default().reasonGetOrCreate(formatted, opts...)
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

// AnyBlockedSoFar reports whether any Task on the default instance is
// currently Blocked — see Output.AnyBlockedSoFar. Package-level parity with
// Task/Group/Print* (beginner-7): a caller using the default-instance
// facade throughout a run should never have to reach for a hosted *Output
// just to check this one thing.
func AnyBlockedSoFar() bool {
	return Default().AnyBlockedSoFar()
}

// AnyFailed reports whether any Task on the default instance is currently
// Failed — see Output.AnyFailed.
func AnyFailed() bool {
	return Default().AnyFailed()
}
