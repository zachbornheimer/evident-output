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

**v0.1-alpha** — semantic core, plain + JSON/JSONL projections, sanitize, roast suite.

| Ready now | Later (see architecture spec) |
|-----------|--------------------------------|
| Items, Task, Tasks, Changes, Plan, Line | Interactive live region / spinner (v0.2) |
| Conclusion + exit codes | slog bridge, Suspend (v0.3) |
| Plain, JSON (§25.1), JSONL (§25.2) | MCP + agent review (v0.5–0.6) |
| `testkit` clock/screen stubs | Full PTY / Windows matrix (v0.4) |

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

- `TRACEABILITY.md` — every architecture §31 requirement ID  
- `schema/scenario.v1.json` — declarative scenario dialect  
- `scenarios/*.json` + Go Appendix H tests (`appendix_h_test.go`)

Architecture source: [`docs/architecture/EVIDENT_OUTPUT_ARCHITECTURE_SPEC_v0.3.md`](docs/architecture/EVIDENT_OUTPUT_ARCHITECTURE_SPEC_v0.3.md).

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

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) (DCO sign-off). Red test → green → refactor. Small conventional commits.
