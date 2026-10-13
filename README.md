# Evident Output

Go presentation library for CLI **state, progress, evidence, changes, plans, and
conclusions**. Application code owns execution; package `evo` owns presentation —
so the same call sites render correctly whether stdout is a real terminal or a
log file, and the exit code always matches what the screen just said.

## Install

```bash
go get github.com/zachbornheimer/evident-output@v2.0.0
```

Requires **Go 1.25+**. License: **Apache-2.0**.

## Quickstart

```go
package main

import (
 "context"
 "os"

 evo "github.com/zachbornheimer/evident-output"
)

func main() { os.Exit(evo.Main(run)) } // Main wires SIGINT/SIGTERM and returns the exit code

func run(ctx context.Context, r *evo.Run) error {
 r.Task("check config").Define(checkConfig)

 installs := r.Group("install") // children may overlap
 for _, pkg := range packages {
  installs.Task(pkg).Define(func(ctx context.Context) error { return install(pkg) })
 }

 r.Sequence("publish").After(installs).Build(func(s *evo.Sequence) {
  s.Task("generate").Define(func(ctx context.Context) error {
   return evo.File(ctx, evo.FileSpec{Path: "out.txt", Contents: []byte("ok\n"), Mode: 0o644})
  })
  s.Task("prune").Define(func(ctx context.Context) error {
   spec := evo.EffectSpec{Verb: evo.Delete, Object: "stale branch", Quantity: 2}
   return evo.Effect(ctx, spec, removeStaleBranches) // ledger: "deleted 2 stale branches"
  })
 })
 return nil
}
```

A Task ends by returning: `nil` succeeds, `evo.Fail("lint failed")` fails,
`evo.Refuse("no remote")` stops before any change, `evo.Exclude("protected")`
records that policy skipped it. `Verify` true before `Define` resolves the
Task Satisfied and the callback never runs. `evo.File` and `evo.Exec` are
Satisfied when their inputs and recorded outputs are current. Set
`EVO_OUTPUT=human|plain|json|jsonl` to pick the encoding.

On a real terminal the frame is live and colored. Piped (CI, an agent
harness), the same code prints durable plain lines:

```text
◐ pipeline  go mod download  2/4  resolving modules
✓ pipeline
   ✓ go mod download
   ✓ go generate
   ✓ go test ./...
```

Try it: `go run ./examples/install-pipeline/ --fast`.

## Outcome and exit code

One verdict drives the screen and the process exit code. A Task ends in
exactly one outcome:

| Outcome     | Meaning                                       | Exit code |
| ----------- | --------------------------------------------- | --------- |
| `Succeeded` | ran and succeeded                             | `0`       |
| `Satisfied` | requested state already true; nothing ran     | `0`       |
| `Excluded`  | policy chose not to run it                    | `0`       |
| `Refused`   | a precondition stopped it before any mutation | `1`       |
| `Failed`    | ran and failed                                | `2`       |
| `Cancelled` | interrupted (SIGINT/SIGTERM)                  | `130`     |

The run's exit code is that of its worst Task outcome. Under `Config.DryRun`
every mutation is planned (`[planned]`) and none runs.

## Pick the entity

| Shape        | Use when                                                                       |
| ------------ | ------------------------------------------------------------------------------ |
| **Task**     | One atomic promise; its work is submitted with `Define`                        |
| **Group**    | Independent children; the scheduler may overlap them                           |
| **Sequence** | Children run in declaration order; a failed child leaves later ones NotStarted |

`After` orders any Task, Group or Sequence behind another.

## Learn more

- [`CONTRACT.md`](CONTRACT.md) — the 2.0 usage contract and full exported surface
- [`docs/migration/2.0.md`](docs/migration/2.0.md) — upgrading from 1.x: every removed symbol with its replacement
- [`mcp/docs/reference.md`](mcp/docs/reference.md) — construction, config, lifecycle, severity dialect, evidence capture, platform adapters, vocabulary
- [`mcp/docs/development.md`](mcp/docs/development.md) — mise commands, conformance suite, examples ladder, CLI, machine output, production ANSI driver, testkit
- [`mcp/docs/mcp.md`](mcp/docs/mcp.md) — the `evident-output-mcp` stdio server (Grok, Claude Code, Codex, …)
- [`mcp/docs/teaching-ladder.md`](mcp/docs/teaching-ladder.md) — the ordinary-surface learning order
- [`CONTRIBUTING.md`](CONTRIBUTING.md) — DCO sign-off, red test → green → refactor, small conventional commits
