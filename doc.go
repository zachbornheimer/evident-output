// Package evo is Evident Output: a presentation library for CLI state, progress,
// evidence, changes, plans, messages, actions, and conclusions.
//
// Application code owns execution. Evo owns presentation.
//
//	func main() {
//	    evo.Init(evo.Config{Title: "repo"}) // first statement — arms first paint before any I/O
//	    os.Exit(evo.Main(run))
//	}
//
//	func run() error {
//	    evo.Println("Reading configuration")
//	    evo.Task("working tree").Done()
//	    t := evo.Task("fetch")
//	    output := t.Evidence()
//	    // run.Run(ctx, "git", args, output); t.Fail(..., output.DetailTail()) on error
//	    return nil // Block is a presentation outcome, not a Go error
//	}
//
// Adoption ladder (guess-driven defaults — the naive spelling is the correct one):
//  1. evo.Init(Config) once in main, before any I/O; os.Exit(evo.Main(run)) — dry-run wording,
//     empty-case, and exit codes are all owned; run returns only error.
//  2. Print / Printf / Println / Verbose — start as casually as fmt.
//  3. evo.Task(name) for everything — a check/gate resolved directly (Done/Warn/Block/Fail/Skip,
//     no Phase/Progress call) renders as a fact row; work with Phase/Progress or a mutation verb
//     (Add/Delete/Create/Update/Remove/Write/Push/Record/RecordName) shows a spinner while running —
//     the verb picks [planned] vs [changed] from Config.DryRun; no call site ever flips its own tense.
//     name is a printf format whenever args follow it (evo.Task("build %s", ref)); no args
//     leaves name untouched.
//  4. evo.Task(name).Each(items) for loop progress (absolute, never double-counted).
//     Each takes []string (the item name becomes the live Phase); for any other slice
//     type, drive the same absolute progress with EachN(len(items)) — no []string copy
//     needed just to get a progress bar.
//     .PhaseWriter() as cmd.Stdout so a talkative child's last line becomes the live Phase;
//     Task.Run(cmd) wires an *exec.Cmd through that same capture/phase plumbing in one call
//     and hands back the subprocess error verbatim for the caller to resolve. An item that
//     fails inside the loop body resolves on the loop's own task handle (task.Fail(...); break)
//     — never a second evo.Task declared per item — leaving Progress sealed at the count
//     already reached (release-gate round 6 finding 7).
//  5. evo.Task(name).Skipped(evo.Reason("..."), name) / .Kept(evo.Reason("..."), name) —
//     taxonomy counted and summed, never a bare "skipped N". evo.Reason(name) is a
//     get-or-create lookup on the default instance: the same string at every call site
//     merges into one bucket, so an inline evo.Reason("protected") is always legal —
//     lifting it to a package var is optional, never required for correctness. Individual
//     names render under Config.Verbosity: VerbosityVerbose (see doc there); at the
//     default VerbosityNormal the human line stays the aggregated "! skipped N (...)"
//     count. The names are never dropped — they always live on TaskSnapshot.Skipped/Kept
//     (Output.Snapshot / TaskHandle.Snapshot); the wire JSON document does not carry them.
//  6. evo.Confirm(question, ...) — owns the whole ask-decide-resolve gate (prompt, quiesce,
//     Done/Blocked resolution, exit code). question is verbatim text, not a printf format
//     like Task/Sequence/Reason/Phase/Skip's text — use fmt.Sprintf to build a dynamic question
//     first. Confirm is the one entity-text spelling that stays non-printf (release-gate
//     round 6 finding 4).
//  7. evo.Sequence(name) for named children with derived, auto-lifecycle state.
//  8. task.Fail(summary) / task.Block(summary) are statements — no return value, so a bare
//     call is errcheck-clean. `return task.Failf("schema mismatch: %w", err)` (task declared
//     as evo.Task("validate manifest")) builds and returns one error in a single line: a
//     trailing ": %w"/", %w" splits the formatted text into the rendered summary and an
//     evidence line for the wrapped error; Blockf is the same for Block. The summary states
//     WHAT went wrong, not the task's own name again — the rendered row already carries the
//     task label, so a summary of "validate manifest: %w" would just repeat it back. Warn,
//     and success/skip verbs, stay void too — this is never fluent chaining.
//     Done/Warn/Task/Sequence/Reason/Phase/Skip are printf-variadic themselves (fmt.Sprintf
//     semantics when args follow); there is no separate Donef/Warnf/Taskf/Reasonf/Phasef/
//     Skipf (C6).
//     Output.Failf stays void rather than mirroring TaskHandle.Failf's *Failure return
//     (release-gate round 4 finding 5): every call site uses it as a bare statement, a
//     returned error would fail errcheck at each of them with no lint-config exception on
//     this repo, and there is no per-call Next chain for an output-level failure to attach
//     to the way TaskHandle.Failf's *Failure attaches to its task (Output.Next already
//     covers the output-level case). Documented asymmetry, not an oversight.
//  9. Config{Debug: evo.DebugConfig{Level: evo.LevelDebug}} selects the journal threshold
//     for Debug/Capture mirrors and the slog bridge. evo.LogLevel is its own type, distinct
//     from stdlib slog.Level — SlogHandler translates between the two internally, but
//     Config.Debug.Level itself never takes a slog.Level value. LevelUnset (the zero value)
//     resolves to LevelInfo; LevelTrace/LevelDebug are the two levels that surface Debug
//     journal lines. Package-level evo.SlogHandler() journals to the default instance,
//     the same default-instance sugar evo.Task/evo.Verbose already offer.
//
// Ordinary surface: evo.Init/evo.Main, Print*, evo.Task/evo.Sequence (+ ID), Task.Evidence,
// Task.Each / Task.PhaseWriter / Task.Run, Task.Fail / Task.Failf / Task.Block / Task.Blockf,
// evo.Confirm, evo.Reason, Changes/Plan (tooling call sites, see below), slog
// via SlogHandler (level from Config.Debug.Level).
//
// Advanced surface, for testing and tooling call sites that need a hosted instance
// instead of the package-level default: Config.Isolated returns an independent *Output
// that never touches package state; Output.Run(run func(*Output) error) seals it (the
// hosted counterpart of Main's run func() error, called on the *Output itself instead
// of the default instance); Config.Options is the raw-Option escape hatch for exact writer/
// terminal/clock wiring. Plan/Changes for the would/did split without a Task, session
// Evidence, terminal drivers, and testkit.
package evo
