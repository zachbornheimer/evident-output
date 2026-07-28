# Teaching ladder (ordinary surface)

Order for learning and documentation. Advanced paths are studio notes, not the lead sheet.

## Ladder

```text
1. New(Config) + Main (standalone) or Finish+Close (hosted)
2. Print / Printf / Println / Verbose
3. Item — gates and conditions
4. Task — work units; Capture on Task or Item
5. Plan / Changes — tense of effect (would / did)
6. ResultWriter or app machine contract (FormatData)
7. Scope — namespaced IDs only
8. Live progress (Tasks collection, Progress/Bytes)
9. slog via SlogHandler (Config.Debug.Level)
10. Advanced: NewWithOptions, terminal drivers, testkit
```

## Standalone

```go
out := evo.New(evo.Config{Title: "tool"})
os.Exit(evo.Main(out, run))
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

| Rule | Meaning |
|------|---------|
| RULE-001 | Domain verbs: `Record("placed", n, noun(...))` not forced `Added` |
| RULE-002 | No vanity Items that restate Plan/Changes |
| RULE-003 | User failures → Item/Task Problems, not slog-only |
| RULE-004 | Predeclare concurrent Tasks before workers |
| RULE-005 | Scale Task cardinality to product need |
| RULE-006 | Capability ≠ obligation |
| PHIL-001 | One ordinary spelling per intent |

See `docs/philosophy/` and `docs/roadmap/implementation-basis.md`.

## Capture

```go
cap := task.Capture() // or item.Capture() for tool-backed gates
// … write to cap.Stdout()/Stderr() …
task.Fail("failed", evo.Cause(err), cap.DetailTail())
```

Pending unterminated fragments are included in DetailTail.

## Data commands

```go
out := evo.New(evo.Config{Title: "tool", Format: evo.FormatData})
json.NewEncoder(out.ResultWriter()).Encode(payload)
```
