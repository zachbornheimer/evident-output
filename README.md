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
Stdin/stdout are JSON-RPC only; logs go to stderr.

**Tools** (namespace `evident_output.*`):

| Tool | Purpose |
|------|---------|
| `evident_output.list_guides` | Guidance catalog (token-budget aware) |
| `evident_output.get_guidance` | Fetch guide sections by id |
| `evident_output.review` | Review Go / package / transcript / structured JSON |
| `evident_output.preview` | Preview plain profiles for a declarative scene |
| `evident_output.explain` | Explain a stable rule id (e.g. `API-006`) |

#### Install the binary

```bash
# From a clone (recommended while developing):
go build -o "$HOME/.local/bin/evident-output-mcp" ./cmd/evident-output-mcp

# Or install the latest public module:
go install github.com/zachbornheimer/evident-output/cmd/evident-output-mcp@latest
# → $(go env GOPATH)/bin/evident-output-mcp  (ensure GOPATH/bin is on PATH)

evident-output-mcp --version
```

Smoke the protocol (each line is one JSON-RPC message on stdin):

```bash
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"manual","version":"0"}}}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' \
  | evident-output-mcp 2>/dev/null | head
```

#### Host config snippets (print-only)

```bash
evident-output-mcp config --client grok         # also: claude-code|codex|gemini|opencode
```

Integrations: [`integrations/`](integrations/) · portable skill: [`skills/cli-output/`](skills/cli-output/)

#### Grok (xAI TUI / Build)

User scope (`~/.grok/config.toml`) — works in every session:

```bash
grok mcp add evident-output -- evident-output-mcp
# or with an absolute path if the binary is not on PATH:
# grok mcp add evident-output -- "$HOME/.local/bin/evident-output-mcp"

# Project scope (this repo already has .grok/config.toml):
# grok mcp add --scope project evident-output -- evident-output-mcp

grok mcp list
grok mcp doctor evident-output
```

Start a **new** Grok session after adding the server so tools are discovered.
See [`integrations/grok/README.md`](integrations/grok/README.md).

Equivalent TOML:

```toml
[mcp_servers.evident-output]
command = "evident-output-mcp"   # or absolute path
enabled = true
startup_timeout_sec = 30
```

Project scope (optional, commit with the repo so teammates get it when the folder is trusted):

```bash
# from the evident-output repo root
mkdir -p .grok
# writes only [mcp_servers] into ./.grok/config.toml
grok mcp add --scope project evident-output -- evident-output-mcp
```

Restart the Grok session (or rely on hot-reload) so tools appear as `evident-output__…` / `search_tool` matches.

#### Claude Code

Add to `~/.claude.json` (or project `.mcp.json`) under `mcpServers`:

```json
{
  "mcpServers": {
    "evident-output": {
      "command": "evident-output-mcp",
      "args": []
    }
  }
}
```

#### Cursor

Settings → MCP → Add server (stdio), command `evident-output-mcp`, no args.
Or project `.cursor/mcp.json` with the same `command` shape as Claude.

#### Manual / any MCP host

```bash
go run github.com/zachbornheimer/evident-output/cmd/evident-output-mcp@latest
# host must speak MCP over stdio; call initialize before tools/*
```

Review kinds for `evident_output.review`: `go` (default), `transcript`, `json` / `structured`.

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
