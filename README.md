# Evident Output

Go presentation library for CLI **state, progress, evidence, changes, plans, and conclusions**.

Application code owns execution. Package `evo` owns presentation only.

## Quick start

```go
import (
    "os"
    evo "github.com/zachbornheimer/evident-output"
)

func main() {
    out := evo.For("bpp-csharp", evo.WriterOptions(os.Stdout)...)
    os.Exit(evo.Main(out, run))
}

func run(out *evo.Output) error {
    out.Item("working tree").OK()
    out.Item("branches").Block(
        "local-only branch",
        evo.Detail("commit or stash before continuing"),
    )
    return nil // Block is a presentation outcome, not a Go error
}
```

```bash
go get github.com/zachbornheimer/evident-output@v0.1.0
# or @latest once tagged
```

Requires **Go 1.25+**. License: **Apache-2.0**.

`evo.Main` owns Finish + Close + exit-code mapping so every binary is not six lines of teardown.  
`evo.WriterOptions(w)` turns on **Plain + NoColor** for non-TTY `*os.File` (pipes/files) so agent log capture stays free of CSI.

## Pick the entity

| Shape | Use when |
|-------|----------|
| **Item** | Check / gate / verdict unit (pass–fail) |
| **Task** | Work with phases or progress |
| **Tasks** | Collection of independent tasks (state is **derived**) |
| **Changes** | Past-tense durable effects that happened |
| **Plan** | Dry-run would-happen effects |

When both Item and Task fit: prefer **Item** for pass/fail gates, **Task** for progress. Multi-gate: resolve every Item, then `if out.AnyBlocked() { return nil }` before mutation; `Main` maps `ExitCode`.

## Severity dialect

| Outcome | Meaning |
|---------|---------|
| **Warn** | Soft concern or **optional** tool missing; command may continue |
| **Block** | Policy / precondition failed; **stop before mutation** (evaluation succeeded) |
| **Fail** | Evaluation failed or **required** tool/IO failed |

`Block` ≠ Go `error`. After Block, return nil from `run` and let `Main` exit `1`.

## Child processes

Capture belongs to the **operation** (`Task`), not the whole session — and not `context`:

```go
upgrade := out.Task("brew packages")
output := upgrade.Capture() // always retains a bounded ring; debug only controls display

if err := run.Run(ctx, "brew", []string{"upgrade", "--formula"}, output); err != nil {
    upgrade.Fail("brew upgrade failed", evo.Cause(err), output.DetailTail())
    return nil
}
upgrade.Done()
```

- **Ownership:** `upgrade.Capture()` associates evidence with that task (debug can label `task=…`).
- **Evidence vs display:** ring is always kept; `DebugLevel` only controls Debug/Diagnostics projection.
- **Detail:** `output.DetailTail()` is a `ProblemOption` (compose with Fail); prefer stderr when streams are split (`output.Stdout()` / `output.Stderr()`).
- **Defaults:** last 200 lines / 256KiB, sanitized, truncation marked; never auto-surfaces on success.
- **`DebugWriter`:** intentional DEBUG journal only — not for child tools.

## Status

**Release:** **v0.1.0** (module path above; no `replace` required for consumers).  
**Architecture spec:** [v0.5](docs/architecture/EVIDENT_OUTPUT_ARCHITECTURE_SPEC_v0.5.md) (design candidate).  
**Implemented surface:** v0.3–v0.4 core (library, interactive VT, debug history/pane, real CLI, hardened MCP, §31 automated rows test-gated). External/manual items remain waived (Windows ConPTY / tmux / SSH RC, a11y contrast / screen-reader, host RC matrices and a11y manual reviews).

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
| `Main` | Finish + Close + process exit code for CLI entrypoints |

Do **not** put schedulers, `RunAll`, retries, or shell execution in this library. Review rule **API-026** flags those helpers only on evo receivers (AST), not `strings.Map`.

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

Architecture source (current design candidate): [`docs/architecture/EVIDENT_OUTPUT_ARCHITECTURE_SPEC_v0.5.md`](docs/architecture/EVIDENT_OUTPUT_ARCHITECTURE_SPEC_v0.5.md).  
Prior implemented baseline: [`docs/architecture/EVIDENT_OUTPUT_ARCHITECTURE_SPEC_v0.3.md`](docs/architecture/EVIDENT_OUTPUT_ARCHITECTURE_SPEC_v0.3.md).

Completeness vs §31 (v0.3 matrix): [`docs/architecture/COMPLETENESS_MATRIX.md`](docs/architecture/COMPLETENESS_MATRIX.md) (**267 pass / 5 waived**).

### CLI

```bash
go run ./cmd/evident-output review path/to/file.go   # JSON findings (exit 1 if recheck_required)
go run ./cmd/evident-output preview --item=status --state=ok
go run ./cmd/evident-output explain API-006
go run ./cmd/evident-output version
```

### MCP (stdio)

The companion server `evident-output-mcp` is a **local stdio** MCP process (no hosted URL).
Stdin/stdout are JSON-RPC only; logs go to **stderr**. Transport supports **NDJSON** (MCP
spec) and **Content-Length** framing (some client SDKs).

**Advertised tool names** use underscores (Grok rejects dotted tool names and then
registers `tool_count: 0`). Dotted aliases still work on `tools/call`.

| Tool name (tools/list) | Grok `use_tool` id | Purpose |
|------------------------|--------------------|---------|
| `evident_output_list_guides` | `evident-output__evident_output_list_guides` | Guidance catalog |
| `evident_output_get_guidance` | `evident-output__evident_output_get_guidance` | Sections by id |
| `evident_output_review` | `evident-output__evident_output_review` | Go / transcript / JSON review |
| `evident_output_preview` | `evident-output__evident_output_preview` | Plain profile previews |
| `evident_output_explain` | `evident-output__evident_output_explain` | Rule id (`rule_id`) |

`explain` arguments: `{ "rule_id": "DOM-011" }` (not `id`).

#### Install the binary (full paths)

```bash
mkdir -p "$HOME/.local/bin"

# Preferred when cloned on this Mac:
go build -o "$HOME/.local/bin/evident-output-mcp" \
  "$HOME/Developer/Personal/evident-output/cmd/evident-output-mcp"

# From any clone (relative only after cd into the repo root):
#   git clone https://github.com/zachbornheimer/evident-output.git
#   cd evident-output && go build -o "$HOME/.local/bin/evident-output-mcp" ./cmd/evident-output-mcp

# Module install (network + sumdb):
#   GOBIN="$HOME/.local/bin" go install \
#     github.com/zachbornheimer/evident-output/cmd/evident-output-mcp@latest

"$HOME/.local/bin/evident-output-mcp" --version
```

Host configs must use an **absolute** command (or `${HOME}/…` where the host expands
it). Bare `evident-output-mcp` fails when the agent process PATH omits `~/.local/bin`.

#### Verify without restarting an existing TUI session

```bash
# Process-level handshake
grok mcp doctor evident-output --json
# expect: healthy=true, "5 tools discovered", protocol 2025-06-18

# Fresh agent process (same attach path as the TUI)
grok -p 'Call use_tool on evident-output__evident_output_list_guides with {}. Reply CONNECTED and the text field, or FAILED.' \
  --output-format plain \
  --max-turns 5 \
  --always-approve \
  --cwd "$HOME/Developer/Personal/evident-output"
# expect: CONNECTED / "5 guides"
```

If doctor is green but headless says FAILED, check session `events.jsonl` for
`mcp_server_connected` with `"tool_count":0` (tool names/schemas rejected) vs
`mcp_server_failed` (spawn/handshake).

NDJSON smoke (no Grok required):

```bash
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"manual","version":"0"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' \
  | "$HOME/.local/bin/evident-output-mcp" 2>/dev/null
```

#### Host config snippets (print-only)

```bash
"$HOME/.local/bin/evident-output-mcp" config --client grok
# also: claude-code|codex|gemini|opencode  — uses ${HOME}/.local/bin/…
```

Integrations: [`integrations/`](integrations/) · skill: [`skills/cli-output/SKILL.md`](skills/cli-output/SKILL.md)

#### Grok (xAI TUI / Build)

```toml
# $HOME/.grok/config.toml  (${HOME} expanded by Grok)
[mcp_servers.evident-output]
command = "${HOME}/.local/bin/evident-output-mcp"
enabled = true
startup_timeout_sec = 30
```

```bash
grok mcp add evident-output -- "$HOME/.local/bin/evident-output-mcp"
grok mcp list
grok mcp doctor evident-output --json
```

**Project scope** only starts when the folder is **trusted**. Prefer user scope for
always-on tools.

See [`integrations/grok/README.md`](integrations/grok/README.md).

#### Claude Code / Cursor / Codex

```bash
"$HOME/.local/bin/evident-output-mcp" config --client claude-code
"$HOME/.local/bin/evident-output-mcp" config --client codex
```

Review kinds for `evident_output_review`: `go` (default), `transcript`, `json` / `structured`.

### Examples

Small real programs (flags, help, exit codes) — not snippets. Copy a whole folder as a starting shape.

| Example | Pattern |
|---------|---------|
| `repo-status` | Parallel **Items** (OK / blocked / warn), conclusion exit code |
| `install-pipeline` | **Tasks** collection with Progress/Bytes/Fail (final report) |
| `migrate` | **Plan** dry-run vs **Changes** apply (`--apply`) |
| `doctor` | Mixed doctor items; `--json` snapshot on stdout |
| `data-command` | Data command: JSON **stdout**, human report **stderr** |
| `live-progress` | **Live multi-progress**: bars + indeterminate phases (ANSI on stderr) |
| `debug-history` | **DebugHistory**: durable `HH:MM:SS [DEBUG] …` above live/items |
| `debug-pane` | **DebugPane**: rolling slog pane; `--fail` keeps diagnostics tail |

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
// DebugHistory (default): out.Debug → durable above live (timestamp + [DEBUG]).
// DebugPane(...): rolling slog viewport in the live region; optional failure tail.
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) (DCO sign-off). Red test → green → refactor. Small conventional commits.
