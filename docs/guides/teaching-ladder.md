# Teaching ladder (ordinary surface)

Order for learning and documentation. Advanced paths are studio notes, not the lead sheet.

## Ladder

```text
1. evo.Init(Config) + evo.Main(run) — arms first paint, owns dry-run wording and exit codes
2. Print / Printf / Println / Verbose
3. Task.Define — one atomic operation; mutation verbs (Delete/Create/Update/…) are the
   dry-run-aware equivalent of Define
4. Group.Each / Sequence.Each — independent vs ordered collections; After for a DAG edge
   nesting cannot express
5. Skipped / Kept — skip/keep taxonomy (reason + name, never a bare count)
6. Confirm — the whole ask-decide-resolve gate
7. ResultWriter or app machine contract (FormatData)
8. slog via SlogHandler (Config.Debug.Level)
9. Advanced: Config.Isolated + Output.Run (hosted instance), terminal drivers, testkit, Suspend

Task's mutation verbs (Delete/Create/…) pick [planned] vs [changed] from Config.DryRun on the
ordinary path — no separate Plan/Changes call site exists to reach for. Quantity is
evo.Affected(n) when one atomic operation touches more than one item.
```

## Standalone (package-level default instance)

```go
func main() {
    evo.Init(evo.Config{Title: "tool"}) // first statement — arms first paint before any I/O
    evo.Main(run)                        // exits the process itself
}

func run() error {
    for path, task := range evo.Group("worktrees").Each(items) {
        task.Define(func() error { return check(path) })
    }
    return nil
}
```

## Hosted (framework owns exit)

`out.Run` returns an `int` (the exit code). The host inspects it and exits.
Do not `return out.Run(run)` from `func main()` — that does not compile.
`evo.Main` is the process-exit path (row 1), not this one.

```go
out := evo.Init(evo.Config{Title: "tool", Isolated: true})
os.Exit(out.Run(run)) // reconciles a non-nil run error into Fail, then Finish
```

## House rules (short)

| Rule     | Meaning                                                           |
| -------- | ----------------------------------------------------------------- |
| RULE-001 | Domain verbs: `Record("placed", n, noun(...))` not forced `Added` |
| RULE-002 | No vanity Tasks that restate the mutation ledger                  |
| RULE-003 | User failures → Task Problems, not slog-only                      |
| RULE-004 | Predeclare concurrent Tasks before workers                        |
| RULE-005 | Scale Task cardinality to product need                            |
| RULE-006 | Capability ≠ obligation                                           |
| PHIL-001 | One ordinary spelling per intent                                  |

Batch elements are one Task with Progress+Doing (count + muted activity), not N Tasks.
Use `TruncateNames` for a single skip/kept list when names must stay readable.

See `docs/philosophy/` and `docs/roadmap/implementation-basis.md`.
Release pin procedure: `docs/guides/cutting-a-release.md`.

## Evidence

```go
cmd.Stdout = task.Writer()
cmd.Stderr = task.Writer()
if err := cmd.Run(); err != nil {
    return task.Failf("failed: %w", err)
}
```

`Writer()` turns the child's last line into live doing-text and retains a bounded ring for Fail evidence.

## Confirm

```go
ok := evo.Confirm("delete origin/production-hotfix?", evo.AssumeYes(flagYes))
```

Owns the whole gate: spinner pause, the `?` prompt, stdin. "n" resolves `⊘ declined`; non-TTY without
`--yes` resolves `⊘ blocked by policy` — never a Go error, never Failed. `question` is the one
non-printf exception on this ladder — it is literal text, not a format string, so build it with
`fmt.Sprintf` first if it needs interpolation. Both outcomes are `Blocked`, so the run concludes
`[blocked]` → exit `1`; pass `AssumeYes` (or gate on your own flag before calling Confirm) if a
decline should exit `0` instead. The blocked-by-policy hint defaults to naming a `--yes` flag your
program may not actually have — pass `evo.PolicyFlag("--apply")` to name the real one:
`evo.Confirm(q, evo.PolicyFlag("--apply"))`.

## Suspend (handing the tty to a child)

```go
cmd := exec.Command("zq", "setup")
cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
out.Suspend(func() error { return cmd.Run() })
```

Only needed when a child paints its own UI on the shared terminal (tty passthrough); a captured or
`Writer`-wired child never needs it.

## Data commands

```go
out := evo.Init(evo.Config{Title: "tool", Format: evo.FormatData, Isolated: true})
json.NewEncoder(out.ResultWriter()).Encode(payload)
```
