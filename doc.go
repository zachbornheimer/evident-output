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
//	    evo.Item("working tree").OK()
//	    t := evo.Task("fetch")
//	    output := t.Capture()
//	    // run.Run(ctx, "git", args, output); t.Fail(..., output.DetailTail()) on error
//	    return nil // Block is a presentation outcome, not a Go error
//	}
//
// Adoption ladder (guess-driven defaults — the naive spelling is the correct one):
//  1. evo.Init(Config) once in main, before any I/O; os.Exit(evo.Main(run)) — dry-run wording,
//     empty-case, and exit codes are all owned; run returns only error.
//  2. Print / Printf / Println / Verbose — start as casually as fmt.
//  3. evo.Item(name) for checks and gates (pass/fail); evo.Task(name) for work with
//     Phase/Progress or a mutation verb (Delete/Create/Update/Remove/Write/Push/Record/RecordName) —
//     the verb picks [planned] vs [changed] from Config.DryRun; no call site ever flips its own tense.
//     name is a printf format whenever args follow it (evo.Task("build %s", ref)); no args
//     leaves name untouched.
//  4. evo.Task(name).Each(items) for loop progress (absolute, never double-counted);
//     .PhaseWriter() as cmd.Stdout so a talkative child's last line becomes the live Phase;
//     Task.Run(cmd) wires an *exec.Cmd through that same capture/phase plumbing in one call
//     and hands back the subprocess error verbatim for the caller to resolve.
//  5. evo.Task(name).Skipped(reason, name) / .Kept(reason, name) — taxonomy counted and
//     summed, never a bare "skipped N".
//  6. evo.Confirm(question, ...) — owns the whole ask-decide-resolve gate (prompt, quiesce,
//     ⊘/OK resolution, exit code).
//  7. evo.Group(name) for named children with derived, auto-lifecycle state.
//  8. task.Fail(summary, evo.Cause(err)) / item.Block(summary, evo.Cause(err)) return one
//     error to `return` directly: message is summary, wrapping Cause with %w so errors.Is/As
//     still reach it; Failf/Blockf take the same summary as a printf format. Success/skip verbs
//     stay void — this is never fluent chaining.
//
// Ordinary surface: evo.Init/evo.Main, Print*, evo.Item/evo.Task/evo.Group (+ ID),
// Task.Capture / Item.Capture, Task.Each / Task.PhaseWriter / Task.Run, Task.Fail / Task.Failf /
// Item.Block / Item.Blockf, evo.Confirm, evo.Reason, Changes/Plan (tooling call sites, see
// below), slog via SlogHandler (level from Config.Debug.Level).
//
// Advanced surface, for testing and tooling call sites that need a hosted instance
// instead of the package-level default: New(Config) / NewWithOptions to construct an
// *Output directly, MainWith(out, run) to seal it (this is what Main calls on the
// default instance), Plan/Changes for the would/did split without a Task, session
// Capture, Progress64, Advance, terminal drivers, and testkit.
package evo
