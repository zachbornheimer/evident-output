# Teaching ladder (ordinary surface)

Order for learning and documentation. Advanced paths are studio notes, not the lead sheet.

## Ladder

```text
1. Task + Define — one atomic operation; Define(fn func(context.Context) error) is the
   scheduling and execution boundary. Inside it, evo.Effect and evo.File are the
   dry-run-aware way to report effects.
2. Group / Sequence — independent vs ordered collections, one named Task per item
   (`Group.Each`/`Sequence.Each` were removed in 1.0); After for a DAG edge nesting
   cannot express.
3. evo.File for declarative managed-state file content.
4. evo.Exec for external work with declared outputs.
5. FileSpec.Inputs / ExecSpec.Inputs (paths) and Values (named values)
   when freshness depends on external inputs beyond File/Exec's own tracking.
6. After for exceptional execution dependencies a Sequence would otherwise express.
7. Facts / warnings / Effects / dry-run — Task.Fact, Task.Problem (non-fatal), Config.DryRun.
8. Task.Verify(func(context.Context) (bool, error)) only for domains Evo cannot track
   automatically — never the default way to make ordinary work idempotent.
9. Top-level Config.Output / Config.Verbose — only when the host CLI needs machine or
   verbose output; never set per Task.

Inside Define, evo.Effect (opaque mutations: a git ref, a worktree, an API change) and evo.File
(file state) pick [planned] vs [changed] from Config.DryRun on the ordinary path — no separate
Plan/Changes call site exists to reach for. EffectSpec.Quantity counts one atomic operation
that touches more than one item.
```

## Standalone (package-level default instance)

```go
func main() {
    os.Exit(evo.Main(evo.Config{}, run))
}

func run(ctx context.Context, r *evo.Run) error {
    worktrees := r.Group("worktrees")
    for _, path := range items {
        worktrees.Task(path).Define(func(ctx context.Context) error { return check(path) })
    }
    return nil
}
```

## Hosted (framework owns exit)

`evo.Execute(ctx, cfg, run)` returns a `Result`; `Result.ExitCode()` is the process exit code.
The host inspects it and exits. `os.Exit(evo.Main(evo.Config{}, run))` is the process-exit path for an
ordinary `main()`; `Execute` is the hosted counterpart, and never exits the process itself.

```go
os.Exit(evo.Execute(ctx, evo.Config{Name: "tool"}, run).ExitCode())
```

## House rules (short)

| Rule     | Meaning                                                                                 |
| -------- | --------------------------------------------------------------------------------------- |
| RULE-001 | True verbs from the closed `Verb` set; the domain noun goes in `Object`, never the verb |
| RULE-002 | No vanity Tasks that restate the mutation ledger                                        |
| RULE-003 | User failures → Task Problems, not slog-only                                            |
| RULE-004 | Predeclare concurrent Tasks before workers                                              |
| RULE-005 | Scale Task cardinality to product need                                                  |
| RULE-006 | Capability ≠ obligation                                                                 |
| PHIL-001 | One ordinary spelling per intent                                                        |

Batch elements are one Task with Progress+Doing (count + muted activity), not N Tasks.
Use `TruncateNames` for a single skip/kept list when names must stay readable.

## Capture

```go
task.Define(func(ctx context.Context) error {
    cmd := exec.CommandContext(ctx, "make", "test")
    cmd.Stdout = task.Writer()
    cmd.Stderr = task.Writer()
    return cmd.Run()
})
```

`Writer()` turns the child's last line into live doing-text and retains a bounded ring for Fail evidence. Run the child inside the Task's `Define`, so its result resolves the row.

## Confirm

```go
ok := r.Confirm("delete origin/production-hotfix?", evo.Destructive(), evo.NonInteractive("--yes"))
```

Owns the whole gate: spinner pause, the `?` prompt, stdin. "n" returns false and resolves `⊘ declined`;
non-TTY without the `NonInteractive` flag is refused — never a Go error. `question` is the one
non-printf exception on this ladder — it is literal text, not a format string, so build it with
`fmt.Sprintf` first if it needs interpolation. Return `evo.Refuse("declined")` after a false answer, so
the run concludes Refused → exit `1`; gate on your own flag before calling Confirm if a decline
should exit `0` instead. `evo.NonInteractive("--yes")` names the flag that passes the gate
unattended, and the refusal's remedy names it, so pass your program's real flag spelling.

## Suspend (handing the tty to a child)

```go
cmd := exec.Command("zq", "setup")
cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
_ = out.Suspend(func() error { return cmd.Run() })
```

Only needed when a child paints its own UI on the shared terminal (tty passthrough); a captured or
`Writer`-wired child never needs it.

## Data commands

```go
_ = evo.Execute(ctx, evo.Config{Name: "tool", Output: evo.JSON}, func(ctx context.Context, r *evo.Run) error {
    return json.NewEncoder(r.ResultWriter()).Encode(payload)
})
```
