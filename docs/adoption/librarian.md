# Adoption case study: librarian

**Evo pin:** v0.2.9+ (presentation polish; library pin hygiene at v0.2.10)  
**Validated mode:** batch-summary only  
**Not validated:** live per-file Tasks under load

## Application shape

Librarian applies a JSONL routing manifest to a source tree (place / offload /
mirror). Domain packages: `internal/broker`, `facade`, `router`, `manifest`.
CLI: `cmd/librarian`. Machine contract: optional `-status` JSON.

## Before

Per-file and summary logging via `slog` JSON on stdout. Dry-run was more log
lines. Exit path separate from presentation story.

## After

```text
domain workers → summary counts + fileFailure records
present.go     → Plan | Changes + optional FailedBy Item
-status JSON   → unchanged machine contract
```

Construction:

```go
out := evo.New(evo.Config{
    Title: "librarian",
    Debug: evo.DebugConfig{Level: evo.LevelWarn},
})
os.Exit(evo.Main(out, run))
```

## Mechanical changes

- Introduced `present.go` (presentation SRP)
- Workers slog WARN/ERROR only; no happy-path Info as UI
- Dry-run → `Plan`; live → `Changes` with domain `Record` verbs
- Path-scoped failures → `FailedBy` (bounded)

## Mistakes discovered (and fixed)

| Mistake | Fix |
|---------|-----|
| `Added(1, "files placed")` | `Record("placed", n, noun(...))` |
| Vanity `✓ dry-run plan` Item | removed |
| slog-only failure | `fileFailure` + FailedBy |
| Duplicate conclusion band | evo DEC-COAL projection (library-side) |

## LOC

Presentation layer grew (~`present.go`); domain stayed free of evo. Metric is
centralization of presentation decisions, not line count.

## What this validates

Config, Main, Plan/Changes, Item severity, slog vs human, machine contract
preservation, post-worker presentation, concurrent workers with summary-only evo.

## What this does not validate

Live concurrent Task updates, thousands of Tasks, per-file Capture, facade
progress callbacks, cancel mid-flight, evo JSON as external contract.

## Why per-file progress was deferred

Placement is often hardlink-fast; summary Changes answer “what happened.”
Capability ≠ obligation (RULE-006).

## Reproduce

```bash
# in librarian repo
go test ./...
# dry-run / live against a temp layout (see cmd/librarian tests)
```

Commits (librarian): `3edd911` → `8eae112` → `e03e6d4`.
