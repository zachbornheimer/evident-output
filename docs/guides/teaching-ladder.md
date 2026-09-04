# Teaching ladder (ordinary surface)

Order for learning and documentation. Advanced paths are studio notes, not the lead sheet.

## Ladder

```text
1. evo.Init(Config) + os.Exit(evo.Main(run)) — arms first paint, owns dry-run wording and exit codes
2. Print / Printf / Println / Verbose
3. Task — everything: a gate/condition resolved directly (Done/Warn/Block/Fail/Skip, no Phase/
   Progress) or work with phases, progress, or mutation verbs (Delete/Create/Update/…); Evidence
   on either shape
4. Each / PhaseWriter — loop progress and child-process narration
5. Skipped / Kept — skip/keep taxonomy (reason + name, never a bare count)
6. Confirm — the whole ask-decide-resolve gate
7. ResultWriter or app machine contract (FormatData)
8. Scope — namespaced IDs only
9. slog via SlogHandler (Config.Debug.Level)
10. Advanced: Config.Isolated + Output.Run (hosted instance), Config.Options (raw-Option escape
    hatch), Plan/Changes (tooling call sites), terminal drivers, testkit, Suspend

Plan/Changes (rung 11) demoted to advanced: Task's mutation verbs (Delete/Create/…) already pick
[planned] vs [changed] from Config.DryRun on the ordinary path; Plan/Changes stay for tooling call
sites that need the instance API directly.
```

## Standalone (package-level default instance)

```go
func main() {
    evo.Init(evo.Config{Title: "tool"}) // first statement — arms first paint before any I/O
    os.Exit(evo.Main(run))
}

func run() error {
    for range evo.Task("scan").Each(items) {
        // scan each item — no explicit Done needed: a completed Each loop
        // auto-resolves Done at Finish, same as a recorded mutation effect.
    }
    return nil
}
```

## Hosted (framework owns exit)

```go
out := evo.Init(evo.Config{Title: "tool", Isolated: true})
defer func() { _ = out.Close() }()
// … use out …
if err != nil && !out.AnyFailed() {
    out.Failf("command failed: %w", err)
}
return out.Finish()
```

## House rules (short)

| Rule     | Meaning                                                           |
| -------- | ----------------------------------------------------------------- |
| RULE-001 | Domain verbs: `Record("placed", n, noun(...))` not forced `Added` |
| RULE-002 | No vanity Items that restate Plan/Changes                         |
| RULE-003 | User failures → Item/Task Problems, not slog-only                 |
| RULE-004 | Predeclare concurrent Tasks before workers                        |
| RULE-005 | Scale Task cardinality to product need                            |
| RULE-006 | Capability ≠ obligation                                           |
| PHIL-001 | One ordinary spelling per intent                                  |

Batch elements are one Task with Progress+Phase (count + muted activity), not N Items.
Use `TruncateNames` for a single skip/kept list when names must stay readable.

See `docs/philosophy/` and `docs/roadmap/implementation-basis.md`.
Release pin procedure: `docs/guides/cutting-a-release.md`.

## Evidence

```go
proof := task.Evidence() // or item.Evidence() for tool-backed gates
// … write to proof.Stdout()/Stderr() …
return task.Failf("failed: %w", err)
```

Pending unterminated fragments are included in DetailTail. Prefer `task.Run(cmd)` for an
`*exec.Cmd` — it wires Evidence and Phase together in one call.

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
`PhaseWriter`-wired child never needs it.

## Data commands

```go
out := evo.Init(evo.Config{Title: "tool", Format: evo.FormatData, Isolated: true})
json.NewEncoder(out.ResultWriter()).Encode(payload)
```
