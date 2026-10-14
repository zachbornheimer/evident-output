// Package evo is Evident Output: a presentation and execution-tracking
// library for CLI state, progress, capture, provenance, and conclusions.
//
// Application code owns declaring work. Evo owns scheduling that work,
// tracking whether it is already satisfied, and presenting the result, so
// the same call sites render correctly whether stdout is a real terminal, a
// log file, or a machine consumer, and the exit code always matches what the
// screen just said. CONTRACT.md lists the whole exported surface.
//
//	func main() { os.Exit(evo.Main(evo.Config{}, run)) }
//
//	func run(ctx context.Context, r *evo.Run) error {
//	    fmt.Fprintln(r.Text(), "Reading configuration")
//	    r.Task("working tree").Define(checkWorkingTree)
//	    status := r.Task("git status")
//	    status.Define(func(ctx context.Context) error {
//	        cmd := exec.CommandContext(ctx, "git", "status", "--short")
//	        cmd.Stdout = status.Writer() // the child's output is this row's capture
//	        cmd.Stderr = status.Writer()
//	        return cmd.Run()
//	    })
//	    return nil
//	}
//
// # Migrating from 1.x
//
// docs/migration/2.0.md maps every removed 1.x spelling to its 2.0
// replacement, and mcp/docs/teaching-ladder.md gives the adoption order.
//
// # Adoption ladder
//
//  1. r.Task(name) + Task.Define(func(context.Context) error) for one atomic
//     unit of work: the scheduling and execution boundary.
//  2. r.Group(name) / r.Sequence(name) for collections: one named child Task
//     per item (group.Task(name)), not a hand-maintained counter. A Group's
//     children may overlap; a Sequence's run in declaration order and a
//     failure leaves later siblings NotStarted. A repeated child name under
//     the same parent is a duplicate sibling declaration.
//  3. evo.File(ctx, evo.FileSpec{...}) for the desired state of one file:
//     Evo creates it, rewrites it on drift, and does nothing when it already
//     holds. ctx comes from a Task's Define callback.
//  4. evo.Exec for external work with declared Outputs; Inputs and Values
//     decide freshness.
//  5. Task.After for scheduler edges a Sequence would not express, and
//     evo.Compute for a Task that produces a value later work reads.
//  6. Task.Fact and Run.Fact for discovered information, Task.Problem for a
//     non-fatal finding, evo.Effect for a mutation Evo cannot model, and
//     Config.DryRun for the planned/changed split.
//  7. Task.Verify(func(context.Context) (bool, error)) only for domains Evo
//     cannot track itself: the one boolean, read-only escape hatch.
//  8. Config.Output and the EVO_OUTPUT environment variable only when the
//     host needs machine output; never per Task.
//
// # Endings
//
// A Task ends exactly once, by returning from its Define callback: nil
// succeeds, evo.Fail ran and broke, evo.Refuse stopped before any mutation,
// and evo.Exclude means policy chose not to run it. Run.Confirm gates
// destructive work.
//
// # Embedding
//
// evo.Main wires SIGINT and SIGTERM, runs, and returns the exit code.
// evo.Execute does the same without signals and returns the full Result,
// for tests and hosts that own the context and the exit.
package evo
