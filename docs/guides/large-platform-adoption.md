# Large-platform adoption

Guidance for Docker-/npm-/Homebrew-scale CLIs integrating Evident Output.

## Architecture triangle

```text
domain / facade     → work + neutral progress callbacks (no evo import)
command layer       → evo entities and presentation
machine contract    → existing -status JSON / ResultWriter / schemas
```

## What to adopt first

1. `Init(Config{Title})` + `Main` or hosted `Output.Run`
2. Mutation verbs (`Delete`/`Create`/`Update`/…) for dry-run vs live — `Config.DryRun` picks
   `[planned]` vs `[changed]` at the same call site
3. Task for real gates; Fail/Block for path-scoped evidence
4. `cmd.Stdout = task.Writer()` (and stderr) for subprocesses
5. Task name only — no `ID` / `Scope` on the ordinary surface
6. `Config.Redactor` before debug retention of secrets

## What not to force

- Per-file Tasks for huge batches — use aggregate Progress (RULE-005)
- Evo as command router or scheduler
- Replacing a stable machine JSON contract with evo human chrome
- Pluralization or localization inside evo

## Concurrent work

Predeclare Tasks in semantic order; workers only update handles. See
`docs/guides/concurrent-progress.md`.

## Porting a reconciler (check/plan/fix/apply)

A reconciler is a CLI shaped like `check` → `plan` → `fix`/`apply`: it
compares desired state against real state, decides what each item needs,
then mutates. homelabctl's `internal/repair` package and `internal/recreate`
package are both this shape. Here is how each piece maps onto evo.

**`Plan()` → `Config{DryRun, Preview}`.** A reconciler's `Plan` builds a
list of actions without touching anything — that's exactly what
`Config.DryRun: true` (a real `--dry-run` run that stops there) or
`Config.Preview: true` (the plan a confirm gate is about to act on) give
you for free. Point the same mutation call — `evo.Effect`, `evo.File`,
`evo.Exec` — at both the check and the apply path; evo picks `[planned]`
vs `[changed]` from `Config`, so `Plan` and `Apply` stop being two code
paths that can drift.

**Already-satisfied checks → `Task.Verify`.** homelabctl's `Converge`
treats "container already matches the compose file" as a silent, no-op
outcome. That's `Task.Verify(func(context.Context) (bool, error))`: when
the check returns true, the Task resolves as already-satisfied and human
output drops it as a landmark row instead of printing a mutation that
never happened.

**Refusals → `Block`.** homelabctl's `Verdict` enum
(`VerdictSafe`/`VerdictRefused` in `internal/repair/plan.go`, mirrored by
`OutcomeRefused` in `internal/recreate/converge.go`) marks an action that
`Plan` decided is unsafe to run — `Apply` then no-ops it without error.
`Block` is the same policy-failed-before-mutation shape: call it when your
own `Plan` equivalent marks an item Refused, then `return nil` and let
`Main` exit 1. A Refused/Blocked item is not a bug — it's Plan's judgment
working.

**Per-item serial loops → `Group`/`Sequence`, ordering via `After`.**
homelabctl's `Apply` walks `[]Action` one at a time and stops the fleet on
the first failure (`TestConverge_PromoteFailureStopsRemainingServices`).
Model that as a `Sequence` with one child `Task` per item, in the order
you need them applied — a `Sequence` keeps its own header and steps, and
each item still gets its own `[planned]`/`[changed]` row. Reach for the
explicit `.After(...)` edge only for the exceptional out-of-order
dependency; a hand-chained run of `.After` calls that just reproduces
linear order belongs in a `Sequence` instead.

Before/after, condensed from homelabctl's `internal/repair` shape:

```go
// Before: hand-rolled plan/apply, Verdict decides who mutates.
actions, _, _ := repair.Plan(findings, fs, docker, env)
results := repair.Apply(actions, docker, launchd, uid)
for _, r := range results {
    if !r.Succeeded() {
        return r.Err
    }
}

// After: each Action becomes a Sequence child; Refused maps to Block.
seq := out.Sequence("repair")
for _, action := range actions {
    t := seq.Task(action.Target)
    if action.Verdict == repair.VerdictRefused {
        t.Block("refused: %s", action.Reason)
        continue
    }
    t.Define(func(ctx context.Context) error {
        return applyOne(ctx, action, docker, launchd, uid)
    })
}
```

### What evo does not own

Porting a reconciler onto evo replaces its _output and control-flow
shape_ — not its domain logic. Evo does not own:

- **Sustained verification** — polling a system after apply to confirm
  the change held; evo reports one run's outcome, not ongoing drift.
- **Rollback or compensation** — undoing a partial apply on failure is
  the reconciler's own responsibility; evo's `Block`/`Fail` stop a run,
  they don't reverse one.
- **Domain history** — evo has no concept of "what changed last run" or
  a diff against a prior state; that's the reconciler's own store.
- **Remote (non-local) resources** — evo's Task/Effect model assumes the
  work happens in this process; a reconciler driving a remote API or
  cluster still owns that transport and its retries itself.

## Case study

`docs/adoption/librarian.md` — batch-summary adoption, mistakes, and limits.
