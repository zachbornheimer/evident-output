# Developing evident-output

Commands, examples, and the lower-level surfaces (CLI, machine output,
production ANSI driver, interactive testkit) for anyone changing the library
itself, not just consuming it.

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

## Conformance (roast)

`conformance/` is the executable specification (Raku/`roast` model):

- `TRACEABILITY.md` — every §31 ID dispositioned (pass / waived with reason + owner for external/manual only; 0 untested)
- `schema/scenario.v1.json` — declarative scenario dialect
- `scenarios/*.json` + Go Appendix H tests (`appendix_h_test.go`)
- `goldens/` — spec-golden byte-for-byte render tests
- `gates/` — release-gate regression rounds

Architecture source (current design candidate): [`architecture/EVIDENT_OUTPUT_ARCHITECTURE_SPEC_v0.5.md`](architecture/EVIDENT_OUTPUT_ARCHITECTURE_SPEC_v0.5.md).
Prior implemented baseline: [`architecture/EVIDENT_OUTPUT_ARCHITECTURE_SPEC_v0.3.md`](architecture/EVIDENT_OUTPUT_ARCHITECTURE_SPEC_v0.3.md).

## Examples (adoption ladder)

```text
examples/print/              Print, Printf, Println
examples/verbose/            visibility gating (--verbose)
examples/repo-status/        Tasks, Problems, actions
examples/install-pipeline/   Tasks + Capture
examples/migrate/            Plan versus Changes
examples/doctor/             severity dialect + WriteJSON
examples/data-command/       machine stdout / human stderr (ResultWriter)
examples/scope-plugin/       Scope + ID for plugin namespaces
examples/live-progress/      ordinary multi-progress
examples/debug-history/      slog durable debug
examples/debug-pane/         rolling slog viewport
examples/terminal-driver/    advanced custom TerminalDriver
```

```bash
mise run examples          # non-interactive batch
go run ./examples/print/
go run ./examples/verbose/ --verbose
```

| Example            | Pattern                                                               |
| ------------------ | --------------------------------------------------------------------- |
| `repo-status`      | Parallel **Tasks** (done / blocked / warn), conclusion exit code      |
| `install-pipeline` | **Tasks** collection with Progress/Bytes/Fail (final report)          |
| `migrate`          | **Plan** dry-run vs **Changes** apply (`--apply`)                     |
| `doctor`           | Mixed doctor items; `--json` snapshot on stdout                       |
| `data-command`     | Data command: JSON **stdout**, human report **stderr**                |
| `live-progress`    | **Live multi-progress**: bars + indeterminate phases (ANSI on stderr) |
| `debug-history`    | **DebugHistory**: durable `HH:MM:SS.mmm [DEBUG] …` above live/items   |
| `debug-pane`       | **DebugPane**: rolling slog pane; `--fail` keeps diagnostics tail     |

```bash
mise run examples                          # all, back-to-back with headers
go run ./examples/repo-status/ --name my-app
go run ./examples/install-pipeline/
go run ./examples/migrate/                 # dry-run plan
go run ./examples/migrate/ --apply
go run ./examples/doctor/ --json | jq .conclusion
go run ./examples/data-command/ 2>/dev/null | jq .
go run ./examples/live-progress/              # in-place ANSI live region (real TTY)
go run ./examples/live-progress/ --frames     # numbered frames you can scroll
go run ./examples/debug-history/              # history-mode debug interleave
go run ./examples/debug-pane/                 # pane removed on success
go run ./examples/debug-pane/ --fail          # failure preserves diagnostics tail

# mise run examples: uses live ANSI when stderr is a TTY; --frames otherwise.
# EVO_EXAMPLES_FRAMES=1 mise run examples   # force scrubable frames in the batch
```

## CLI

```bash
go run ./cmd/evident-output review path/to/file.go   # JSON findings (exit 1 if recheck_required)
go run ./cmd/evident-output preview --item=status --state=ok
go run ./cmd/evident-output explain API-006
go run ./cmd/evident-output version
```

## Machine output

```go
snap := out.Snapshot()
plain, _ := evo.RenderPlain(snap, evo.PlainOptions{Width: 80})
jsonBytes, _ := evo.EncodeJSON(snap)
jsonl, _ := evo.EncodeJSONL(out.Events())
```

Schemas: `../schema/output.v1.json`, `../schema/event.v1.json`.

## Production ANSI driver

```go
import "github.com/zachbornheimer/evident-output/terminal"

drv := terminal.NewANSI(os.Stderr, terminal.WithInteractive(true), terminal.WithSize(80, 24))
out := evo.Init(evo.Config{Title: "deploy", Options: []evo.Option{evo.Terminal(drv)}})
```

No `To()` needed: the driver owns rendering, and evident-output detects its
`Sink()` and routes the residual/plain projection there too — the conclusion
band renders exactly once, never a second time on a different stream.

## Interactive (testkit / virtual terminal)

```go
screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
clock := testkit.NewClock()
out := evo.Init(evo.Config{Options: []evo.Option{
    evo.Terminal(screen),
    evo.Clock(clock),
    evo.VisibilityDelay(150 * time.Millisecond),
    evo.MaxFrameRate(20),
}})
// Phase/Progress draw a live region; instant Done before the threshold does not flash.
// DebugHistory (default): out.Debug → durable above live (timestamp + [DEBUG]).
// DebugPane(...): rolling slog viewport in the live region; optional failure tail.
```
