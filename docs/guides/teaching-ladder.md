# Teaching ladder (ordinary surface)

Order for learning and documentation. Advanced paths are studio notes, not the lead sheet.

## Ladder

```text
1. evo.Init(Config) + os.Exit(evo.Main(run)) — arms first paint, owns dry-run wording and exit codes
2. Print / Printf / Println / Verbose
3. Item — gates and conditions
4. Task — work units; mutation verbs (Delete/Create/Update/…); Capture on Task or Item
5. Each / PhaseWriter — loop progress and child-process narration
6. Skipped / Kept — skip/keep taxonomy (reason + name, never a bare count)
7. Confirm — the whole ask-decide-resolve gate
8. ResultWriter or app machine contract (FormatData)
9. Scope — namespaced IDs only
10. slog via SlogHandler (Config.Debug.Level)
11. Advanced: NewWithOptions, Plan/Changes (tooling call sites), terminal drivers, testkit, Suspend

Rung 5 (Plan/Changes) demoted to advanced: Task's mutation verbs (Delete/Create/…) already pick
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
    evo.Task("scan").Each(items)
    return nil
}
```

## Hosted (framework owns exit)

```go
out := evo.New(evo.Config{Title: "tool"})
defer func() { _ = out.Close() }()
// … use out …
if err != nil && !out.AnyFailed() {
    out.Fail("command failed", evo.Cause(err))
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

## Capture

```go
cap := task.Capture() // or item.Capture() for tool-backed gates
// … write to cap.Stdout()/Stderr() …
task.Fail("failed", evo.Cause(err), cap.DetailTail())
```

Pending unterminated fragments are included in DetailTail.

## Confirm

```go
ok := evo.Confirm("delete origin/production-hotfix?", evo.AssumeYes(flagYes))
```

Owns the whole gate: spinner pause, the `?` prompt, stdin. "n" resolves `⊘ declined`; non-TTY without
`--yes` resolves `⊘ blocked by policy` — never a Go error, never Failed.

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
out := evo.New(evo.Config{Title: "tool", Format: evo.FormatData})
json.NewEncoder(out.ResultWriter()).Encode(payload)
```
