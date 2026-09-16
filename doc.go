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
//	func run(ctx context.Context) error {
//	    evo.Println("Reading configuration")
//	    evo.Task("working tree").Done()
//	    t := evo.Task("fetch")
//	    cmd.Stdout = t.Writer()
//	    cmd.Stderr = t.Writer()
//	    return nil // Block is a presentation outcome, not a Go error
//	}
//
// Adoption ladder (guess-driven defaults — the naive spelling is the correct one):
//  1. evo.Init(Config) once in main, before any I/O; os.Exit(evo.Main(run)) — dry-run
//     wording, empty-case, and exit codes are all owned; run takes a context.Context
//     (wired to SIGINT/SIGTERM) and returns only error; Main returns the derived exit
//     code and does not itself call os.Exit.
//  2. Print / Printf / Println / Verbose — start as casually as fmt.
//  3. evo.Task(name) for everything — a check/gate resolved directly (Done/Warn/Block/Fail/Skip,
//     no Doing/Progress call) renders as a fact row; work with Doing/Progress or a mutation verb
//     (Add/Delete/Create/Update/Remove/Write/Push/Record/RecordName) shows a spinner while running —
//     the verb picks [planned] vs [changed] from Config.DryRun; no call site ever flips its own tense.
//     name is a printf format whenever args follow it (evo.Task("build %s", ref)); no args
//     leaves name untouched. Define(fn) or a mutation verb submits work; Done is only for
//     already-resolved work with no callback.
//  4. evo.Group(name) / evo.Sequence(name) for collection work: declare one child Task per
//     item with a distinct name (evo.Group("install").Task(pkg.Name)); Define or a mutation
//     verb submits it, and evo.Run/evo.Main wait for every submitted Task to settle. A
//     repeated child name under the same parent is a duplicate sibling declaration, not a
//     get-or-create (§3.1) — a caller that wants to keep using one declaration keeps the
//     *TaskHandle it got. cmd.Stdout = task.Writer() so a talkative child's last line becomes
//     the live doing-text. A failed item Fails that child Task — not a second Task declared
//     for the same item.
//  5. evo.Task(name).Skipped(evo.Reason("...")) / .Kept(evo.Reason("...")) —
//     taxonomy counted and summed, never a bare "skipped N". evo.Reason(name) is a
//     get-or-create lookup on the default instance: the same string at every call site
//     merges into one bucket, so an inline evo.Reason("protected") is always legal —
//     lifting it to a package var is optional, never required for correctness. Verbose taxonomy
//     detail (a per-task Skipped/Kept cause list) renders under Config.Verbosity:
//     VerbosityVerbose (see doc there); the per-task "! skipped 1 (...)" / "! kept 1 (...)"
//     line itself is present at every verbosity. The disposition is never dropped — it always
//     lives on TaskSnapshot.Skipped/Kept (Output.Snapshot / TaskHandle.Snapshot); the wire
//     JSON document does not carry it.
//  6. evo.Confirm(question, ...) — owns the whole ask-decide-resolve gate (prompt, quiesce,
//     Done/Blocked resolution, exit code). question is verbatim text, not a printf format
//     like Task/Sequence/Reason/Doing/Skip's text — use fmt.Sprintf to build a dynamic question
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
//     Done/Warn/Task/Sequence/Reason/Doing/Skip are printf-variadic themselves (fmt.Sprintf
//     semantics when args follow); there is no separate Donef/Warnf/Taskf/Reasonf/Doingf/
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
// Ordinary surface: evo.Init/evo.Main, Print*, evo.Task/evo.Group/evo.Sequence,
// Task.Define / mutation verbs / Task.Writer, Task.Fail / Task.Failf / Task.Block / Task.Blockf,
// evo.Confirm, evo.Reason, slog via SlogHandler (level from Config.Debug.Level).
//
// Advanced surface, for testing and tooling call sites that need a hosted instance
// instead of the package-level default: Config.Isolated returns an independent *Output
// that never touches package state; Output.Run(ctx context.Context, run RunFunc) Result
// seals it (the hosted counterpart of Main, returning the full Result — Conclusion plus
// the application error — instead of just the derived exit code, and never exiting the
// process) — Task/Group/Sequence declare on that *Output directly since run no longer
// receives one; Config.Options is the raw-Option escape hatch for exact writer/
// terminal/clock wiring. Plan/Changes for the would/did split without a Task, session
// evidence, terminal drivers, and testkit.
package evo
