# Evident Output

Go presentation library for CLI **state, progress, evidence, changes, plans, and conclusions**.

Application code owns execution. Package `evo` owns presentation only.

## Quick start

```go
import evo "github.com/zachbornheimer/evident-output"

out := evo.For("bpp-csharp")
defer out.Close()

out.Item("working tree").OK()
out.Item("branches").Block(
    "local-only branch",
    evo.Detail("commit or stash before continuing"),
)
return out.Finish() // also try: out.Task / out.Tasks / out.Changes / out.Plan
```

```bash
go get github.com/zachbornheimer/evident-output@latest
```

Requires **Go 1.25+**. License: **Apache-2.0**.

## Status

**v0.3 architecture implemented** — core library, interactive VT, logs, real CLI, hardened MCP, and §31 automated rows are test-gated. Only **external host/manual** items remain waived (Windows ConPTY / tmux / SSH RC, a11y contrast / screen-reader, host RC matrices and a11y manual reviews).

| Ready now | External / manual only |
|-----------|------------------------|
| Items, Task, Tasks, Changes, Plan, Line | Windows ConPTY RC (PORT-003) |
| Conclusion + exit codes + Cancel cleanup | tmux RC (PORT-004) |
| Plain, JSON (§25.1), JSONL (§25.2) | SSH RC (PORT-005) |
| Interactive live region (`testkit.Screen`) | Light/dark contrast review (A11Y-006) |
| `SlogHandler`, `DebugWriter`, `Suspend`, `Snapshots()`, `MaxEntities`, `MaxEvents`, `AlsoWrite` | Screen-reader review (A11Y-007) |
| Appendix H.1–H.22 + agent harness + multi-file GoPackage review | — |
| ANSI driver + width/CJK + OSC strip + s390x cross-compile | — |
| CLI: `review` / `preview` / `explain` (real JSON) | — |
| MCP: lifecycle, protocol negotiate, unknown-field reject, panic contain, token budget, remote-path reject, catalog checksum | — |
| Framework adapter examples (urfave/Kong shapes, no core deps) | — |

## Vocabulary

| Type | Meaning |
|------|---------|
| `Item` | Named condition that stays in the final report |
| `Task` | One operation (phase / progress / done) |
| `Tasks` | Collection of independent tasks (state is **derived**) |
| `Problem` | Structured evidence for warn / block / fail |
| `Changes` / `Plan` | Effects that happened vs would happen |
| `Conclusion` | Headline + `Changed` / `Partial` / `Cancelled` + exit code |

Do **not** put schedulers, `RunAll`, retries, or shell execution in this library.

## Develop

```bash
mise run setup    # go mod download
mise run test     # unit + roast
mise run test-race
mise run conformance
mise run traceability   # all §31 IDs present
mise run ci             # lint + test + scan + conformance + traceability
```

Trunk is configured **daemonless** (`--monitor=false`). Prefer `mise` over raw tools.

### Conformance (roast)

`conformance/` is the executable specification (Raku/`roast` model):

- `TRACEABILITY.md` — all **272** §31 IDs dispositioned (**267 pass**, **5 waived** with reason + owner for external/manual only; **0 untested**)  
- `schema/scenario.v1.json` — declarative scenario dialect  
- `scenarios/*.json` + Go Appendix H tests (`appendix_h_test.go`)

Architecture source: [`docs/architecture/EVIDENT_OUTPUT_ARCHITECTURE_SPEC_v0.3.md`](docs/architecture/EVIDENT_OUTPUT_ARCHITECTURE_SPEC_v0.3.md).

Completeness vs §31: [`docs/architecture/COMPLETENESS_MATRIX.md`](docs/architecture/COMPLETENESS_MATRIX.md) (**267 pass / 5 waived**).

### CLI

```bash
go run ./cmd/evident-output review path/to/file.go   # JSON findings (exit 1 if recheck_required)
go run ./cmd/evident-output preview --item=status --state=ok
go run ./cmd/evident-output explain API-006
go run ./cmd/evident-output version
```

### MCP (stdio)

```bash
go run ./cmd/evident-output-mcp
# initialize first, then tools/list / tools/call (evident_output.*)
# review kinds: go (default), transcript, json|structured
```

### Example

```bash
go run ./examples/repository-item/
```

### Machine output

```go
snap := out.Snapshot()
plain, _ := evo.RenderPlain(snap, evo.PlainOptions{Width: 80})
jsonBytes, _ := evo.EncodeJSON(snap)
jsonl, _ := evo.EncodeJSONL(out.Events())
```

Schemas: `schema/output.v1.json`, `schema/event.v1.json`.

### Production ANSI driver

```go
import "github.com/zachbornheimer/evident-output/terminal"

drv := terminal.NewANSI(os.Stderr, terminal.WithInteractive(true), terminal.WithSize(80, 24))
out := evo.New(evo.Terminal(drv))
```

### Interactive (testkit / virtual terminal)

```go
screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
clock := testkit.NewClock()
out := evo.New(
    evo.Terminal(screen),
    evo.Clock(clock),
    evo.VisibilityDelay(150*time.Millisecond),
    evo.MaxFrameRate(20),
)
// Phase/Progress draw a live region; instant Done before the threshold does not flash.
// out.Debug(...) inserts above the live region (clear → durable → redraw).
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) (DCO sign-off). Red test → green → refactor. Small conventional commits.
