// Package evo is Evident Output: a presentation and execution-tracking
// library for CLI state, progress, evidence, provenance, and conclusions.
//
// Application code owns declaring work. Evo owns scheduling that work,
// tracking whether it is already satisfied, and presenting the result —
// so the same call sites render correctly whether stdout is a real
// terminal, a log file, or a machine consumer, and the exit code always
// matches what the screen just said.
//
// Runnable examples for every concept below live alongside this file in
// example_1_0_test.go: ExampleInit, ExampleRun, ExampleMain, ExampleTask,
// ExampleGroup, ExampleSequence, ExampleTaskHandle_Define,
// ExampleTaskHandle_Verify, ExampleTaskHandle_Key, ExampleFile, and
// ExampleFact — go doc / pkg.go.dev attach each to the symbol it names.
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
// # Migrating from 0.5
//
// evo.MainWith and Task/Group/Sequence.Each were removed in 1.0.0. See
// docs/migration/1.0.md for every breaking change with before/after code,
// and docs/guides/teaching-ladder.md for the current adoption order.
//
// # Adoption ladder (spec §44 — guess-driven defaults, the naive spelling is correct)
//
//  1. evo.Task(name) + Task.Define(func(context.Context) error) for one atomic
//     unit of work — the scheduling and execution boundary (§7). A Task resolved
//     directly with no Define call (Done/Warn/Block/Fail/Skipped) renders as a
//     fact row instead of a spinner.
//  2. evo.Group(name) / evo.Sequence(name) for collections: one named child
//     Task per item (group.Task(name)), not a hand-maintained counter — Group's
//     children may overlap, Sequence's run in declaration order and cascade a
//     failure to NotStarted for later siblings. Group.Each and Sequence.Each
//     were removed in 1.0.0; a repeated child name under the same parent is a
//     duplicate sibling declaration, not a get-or-create (§3.1).
//  3. evo.File(ctx, evo.FileSpec{...}) for declarative managed-state file
//     content — Evo creates, rewrites on drift, and no-ops when the desired
//     state already holds. ctx must come from a Task's Define callback.
//  4. evo.Exec for external work with declared outputs and a Basis of
//     Fingerprints that determine freshness (planned; not yet implemented).
//  5. A Task-level Basis of evo.Fingerprint values (evo.FSPath, evo.Value,
//     evo.App) when an operation's freshness depends on semantic external
//     inputs beyond File/Exec's own tracked state.
//  6. Task.After for exceptional scheduler edges that a Sequence would
//     otherwise express more simply.
//  7. Task.Fact / evo.Fact for discovered information, Task.Warn for a
//     non-terminal annotation, Output Effects, and Config.DryRun for the
//     would/did split.
//  8. Task.Verify(func(context.Context) (bool, error)) only for domains Evo
//     cannot track automatically — the one boolean, read-only escape hatch; a
//     Verify that reports the desired state already holds skips Define and
//     resolves ResolutionAlreadySatisfied.
//  9. Top-level Config.Format / Config.Verbosity only when the host CLI needs
//     machine output or verbose detail — never set per Task.
//
// Do not require named Evidence declarations (a legacy mutating callback
// registered under that name) for common resources; evo.File/evo.Exec cover
// the ordinary cases without one.
//
// # Ordinary surface
//
// evo.Init/evo.Main/evo.Run, Output.Run for a hosted/Isolated instance,
// Print*, evo.Task/evo.Group/evo.Sequence, Task.Define / mutation verbs /
// Task.Writer, Task.Fail / Task.Failf / Task.Block / Task.Blockf,
// evo.Confirm, evo.Reason, slog via SlogHandler (level from Config.Debug.Level).
//
// # Advanced surface
//
// For testing and tooling call sites that need a hosted instance instead of
// the package-level default: Config.Isolated returns an independent *Output
// that never touches package state; Output.Run(ctx, run) Result seals it
// (the hosted counterpart of Main, returning the full Result — Conclusion
// plus the application error — instead of just the derived exit code, and
// never exiting the process). Task.Key overrides a Task's stable identity
// when its name changes across runs but tracked state must not (§3.1).
// Task.Verify, evo.File, and Fingerprint (evo.FSPath/evo.Value/evo.App) are
// the tracked-state primitives; Config.AppID/Config.StateDir override the
// manifest's storage location and namespace.
package evo
