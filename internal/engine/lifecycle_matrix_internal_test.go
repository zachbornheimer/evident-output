package engine

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/wire"
)

// TestLifecycleOutcomeMatrix pins ZYS-1190's "only lifecycle sets a Task
// terminal outcome" contract end to end through the public engine path,
// across every combination of {DryRun,Preview} mode, whether a registered
// Verify is satisfied, and what the Task's Define callback does
// (Skipped, Block, a committed Effect, or a plain no-op success). This is
// the oracle the lifecycle extraction (ZYS-1190 steps 2-3) must not move a
// single row of: the settled state and the run's exit code stay exactly
// what this test pins for every cell (TestLifecycleOutcomeMatrixHasProblems
// and TestLifecycleOutcomeMatrixCancelled below additionally assert the
// rendered ledger text for the axes they cover).
func TestLifecycleOutcomeMatrix(t *testing.T) {
	type body func(t *TaskHandle) func(context.Context) error

	// bodies is the {Skipped, Block, Effect, no-op} axis: each returns a
	// Define callback for a Task already obtained via out.Task(name).
	bodies := map[string]body{
		"skipped": func(task *TaskHandle) func(context.Context) error {
			return func(ctx context.Context) error {
				task.Skipped(Reason("n/a"))
				return nil
			}
		},
		"block": func(task *TaskHandle) func(context.Context) error {
			return func(ctx context.Context) error {
				task.Block("blocked by policy")
				return nil
			}
		},
		"effect": func(task *TaskHandle) func(context.Context) error {
			return func(ctx context.Context) error {
				return Effect(ctx, EffectSpec{Verb: EffectDelete, Object: "branch", Quantity: 1}, func(context.Context) error { return nil })
			}
		},
		"noop": func(task *TaskHandle) func(context.Context) error {
			return func(ctx context.Context) error { return nil }
		},
	}

	// wantState is what each body settles to, independent of mode/verify,
	// except where noted per case below.
	wantState := map[string]EntityState{
		"skipped": Skipped,
		"block":   Blocked,
		"effect":  Done,
		"noop":    Done,
	}

	for _, dryRun := range []bool{false, true} {
		for _, preview := range []bool{false, true} {
			if dryRun && preview {
				// DryRun and Preview are mutually exclusive planning modes
				// (see construct_options.go); not a cell of this matrix.
				continue
			}
			for _, verifySatisfied := range []bool{true, false} {
				for name, makeBody := range bodies {
					t.Run(matrixCaseName(dryRun, preview, verifySatisfied, name), func(t *testing.T) {
						var buf strings.Builder
						out := Init(Config{
							Isolated: true, Plain: true, Color: ColorNever,
							DryRun: dryRun, Preview: preview,
							Stdout: &buf, Stderr: io.Discard,
						})

						var snap TaskSnapshot
						result := out.Run(context.Background(), func(ctx context.Context) error {
							task := out.Task(name)
							task.Verify(func(context.Context) (bool, error) { return verifySatisfied, nil })
							task.Define(makeBody(task))
							_ = task.Wait()
							snap = task.Snapshot()
							return nil
						})

						want := wantState[name]
						switch {
						case verifySatisfied:
							// A pre-satisfied Verify resolves the Task
							// itself, before the (registered) body ever
							// runs (runDefine's allSatisfied branch) —
							// every body in this matrix settles Done in
							// that case, since the callback that would
							// otherwise skip/block/mutate never executes.
							want = Done
						case !verifySatisfied && name == "noop":
							// hasPostStateToVerify is true: no mutation was
							// planned to skip, so checkAfterDefine re-runs
							// the still-unsatisfied Verify and fails the
							// Task (ProblemCodeVerificationUnsatisfied).
							want = Failed
						case !verifySatisfied && name == "effect" && !dryRun && !preview:
							// A live run's committed Effect leaves a real
							// post-state to verify; dry-run/preview skip
							// the mutation Define planned, so the observed
							// state is the one before the plan and the
							// After phase stays unevaluated
							// (hasPostStateToVerify), leaving the Task Done.
							want = Failed
						}
						if snap.State != want {
							t.Fatalf("state = %s, want %s (dryRun=%v preview=%v verifySatisfied=%v body=%s)",
								snap.State, want, dryRun, preview, verifySatisfied, name)
						}

						wantExit := core.ExitOK
						switch want {
						case Blocked:
							wantExit = core.ExitBlocked
						case Failed:
							wantExit = core.ExitFailed
						}
						if exit := result.ExitCode(); exit != wantExit {
							t.Fatalf("exit = %d, want %d (dryRun=%v preview=%v verifySatisfied=%v body=%s); output:\n%s",
								exit, wantExit, dryRun, preview, verifySatisfied, name, buf.String())
						}
					})
				}
			}
		}
	}
}

// TestLifecycleOutcomeMatrixHasProblems extends TestLifecycleOutcomeMatrix
// with the hasProblems axis: a Task carrying a Problem (TaskHandle.Problem)
// before it resolves success-class (Done via a no-op body, Skipped via the
// taxonomy verb) must settle Failed per lifecycle.decide, applied inside Settle, whichever verb
// the body called — the evidence rule engine/lifecycle owns, exercised end
// to end through the public engine path rather than lifecycle's own pure
// table.
func TestLifecycleOutcomeMatrixHasProblems(t *testing.T) {
	cases := []struct {
		name string
		body func(task *TaskHandle) func(context.Context) error
	}{
		{"noop", func(task *TaskHandle) func(context.Context) error {
			return func(context.Context) error { return nil }
		}},
		{"skipped", func(task *TaskHandle) func(context.Context) error {
			return func(context.Context) error {
				task.Skipped(Reason("n/a"))
				return nil
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf strings.Builder
			out := Init(Config{
				Isolated: true, Plain: true, Color: ColorNever,
				Stdout: &buf, Stderr: io.Discard,
			})

			var snap TaskSnapshot
			result := out.Run(context.Background(), func(ctx context.Context) error {
				task := out.Task("with problem")
				task.Problem("something looked wrong")
				task.Define(tc.body(task))
				_ = task.Wait()
				snap = task.Snapshot()
				return nil
			})

			if snap.State != Failed {
				t.Fatalf("state = %s, want failed (a success-class verb over a Task holding a Problem must settle Failed); output:\n%s", snap.State, buf.String())
			}
			if exit := result.ExitCode(); exit != core.ExitFailed {
				t.Fatalf("exit = %d, want %d", exit, core.ExitFailed)
			}
			if !strings.Contains(buf.String(), "something looked wrong") {
				t.Fatalf("rendered ledger missing the Problem's summary; output:\n%s", buf.String())
			}
		})
	}
}

// TestLifecycleOutcomeMatrixCancelled covers the Cancelled axis
// TestLifecycleOutcomeMatrix's body table has no cell for: a Task the
// program cancels directly (TaskHandle.Cancel), and the two synthetic
// Output-level paths (Fail, Cancel) that never have a Running/Pending
// phase of their own — both settle straight from lifecycle.Declared()
// through State.Settle, with no SettledFailed/SettledCancelled
// constructor; lifecycle exposes no such shortcut.
func TestLifecycleOutcomeMatrixCancelled(t *testing.T) {
	t.Run("task cancel", func(t *testing.T) {
		var buf strings.Builder
		out := Init(Config{Isolated: true, Plain: true, Color: ColorNever, Stdout: &buf, Stderr: io.Discard})
		var snap TaskSnapshot
		result := out.Run(context.Background(), func(ctx context.Context) error {
			task := out.Task("interrupted")
			task.Define(func(context.Context) error {
				task.Cancel("interrupted by caller")
				return nil
			})
			_ = task.Wait()
			snap = task.Snapshot()
			return nil
		})
		if snap.State != Cancelled {
			t.Fatalf("state = %s, want cancelled; output:\n%s", snap.State, buf.String())
		}
		if exit := result.ExitCode(); exit != core.ExitCancelled {
			t.Fatalf("exit = %d, want %d", exit, core.ExitCancelled)
		}
		if !strings.Contains(buf.String(), "interrupted") {
			t.Fatalf("rendered ledger missing the cancelled task; output:\n%s", buf.String())
		}
	})

	t.Run("output-level synthetic fail", func(t *testing.T) {
		var stdout nopFlushWriter
		out := Init(Config{Isolated: true, Format: FormatJSONL, Subject: "tool", Stdout: &stdout})
		out.Fail("could not start")
		if err := out.Finish(); err != nil {
			t.Fatalf("Finish: %v", err)
		}
		events := decodeWireEvents(t, stdout.String())
		idx := indexOfType(events, wire.EventRunFinished)
		if idx == -1 {
			t.Fatalf("run.finished must fire on a synthetic Fail, got: %v", wireEventTypes(events))
		}
		if events[idx].Payload["outcome"] != wire.OutcomeFailed {
			t.Fatalf("run.finished outcome = %v, want %q", events[idx].Payload["outcome"], wire.OutcomeFailed)
		}
	})

	t.Run("output-level synthetic cancel", func(t *testing.T) {
		var stdout nopFlushWriter
		out := Init(Config{Isolated: true, Format: FormatJSONL, Subject: "tool", Stdout: &stdout})
		out.Cancel("interrupted by signal")
		if err := out.Finish(); err != nil {
			t.Fatalf("Finish: %v", err)
		}
		events := decodeWireEvents(t, stdout.String())
		idx := indexOfType(events, wire.EventRunFinished)
		if idx == -1 {
			t.Fatalf("run.finished must fire on a synthetic Cancel, got: %v", wireEventTypes(events))
		}
		if events[idx].Payload["outcome"] != wire.OutcomeCancelled {
			t.Fatalf("run.finished outcome = %v, want %q", events[idx].Payload["outcome"], wire.OutcomeCancelled)
		}
	})
}

func matrixCaseName(dryRun, preview, verifySatisfied bool, body string) string {
	mode := "live"
	switch {
	case dryRun:
		mode = "dryrun"
	case preview:
		mode = "preview"
	}
	verify := "unsatisfied"
	if verifySatisfied {
		verify = "satisfied"
	}
	return mode + "/" + verify + "/" + body
}
