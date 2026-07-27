---
name: cli-output
description: >
  Use when creating, modifying, reviewing, testing, or debugging command-line
  output. Apply when a CLI prints items, tasks, multiple progress bars, plans,
  changes, warnings, errors, debug logs, tables, structured output, or next
  actions; or when stdout, stderr, TTY behavior, color, terminal width, CI
  output, exit codes, or live rendering are involved.
license: Apache-2.0
---

# CLI Output

Portable skill for understandable CLI presentation. Prefer **Evident Output**
when available; stay useful when it is not.

## Canonical locations (do not invent paths)

| What | Path |
|------|------|
| **GitHub repo** | `https://github.com/zachbornheimer/evident-output` |
| **Go module** | `github.com/zachbornheimer/evident-output` |
| **MCP package** | `github.com/zachbornheimer/evident-output/cmd/evident-output-mcp` |
| **CLI package** | `github.com/zachbornheimer/evident-output/cmd/evident-output` |
| **This skill in-repo** | `skills/cli-output/SKILL.md` (repo root) |
| **Local clone (this Mac)** | `$HOME/Developer/Personal/evident-output` |
| **MCP binary (install target)** | `$HOME/.local/bin/evident-output-mcp` |
| **Grok user config** | `$HOME/.grok/config.toml` |

Relative `./cmd/…` paths only work **from the repo root** of a checkout. Prefer
**module paths** or the **absolute clone path** above.

## Capability fallback

1. **Connected MCP** — tools in the table below
2. **Standalone CLI** — `go run github.com/zachbornheimer/evident-output/cmd/evident-output@latest …` or installed `evident-output`
3. **This skill’s static guidance** — no MCP/CLI required

Do not fail merely because MCP is offline. Report reduced verification strength when using static guidance only.

## MCP tool names (underscores only)

Grok rejects dotted tool names (`evident_output.list_guides`) and then reports
`tool_count: 0`. Use underscores. On Grok, tools are `server__tool`.

| tools/list | Grok `use_tool` | Purpose |
|------------|-----------------|---------|
| `evident_output_list_guides` | `evident-output__evident_output_list_guides` | Catalog |
| `evident_output_get_guidance` | `evident-output__evident_output_get_guidance` | Sections by id |
| `evident_output_review` | `evident-output__evident_output_review` | Go / transcript / JSON |
| `evident_output_preview` | `evident-output__evident_output_preview` | Plain profiles |
| `evident_output_explain` | `evident-output__evident_output_explain` | `rule_id` (not `id`) |

Example: `explain` body `{ "rule_id": "DOM-011" }`.

## Install MCP binary (full paths)

```bash
mkdir -p "$HOME/.local/bin"

# Preferred on this machine when the repo is already cloned:
go build -o "$HOME/.local/bin/evident-output-mcp" \
  "$HOME/Developer/Personal/evident-output/cmd/evident-output-mcp"

# Or from a fresh clone:
#   git clone https://github.com/zachbornheimer/evident-output.git
#   cd evident-output
#   go build -o "$HOME/.local/bin/evident-output-mcp" ./cmd/evident-output-mcp

# Or module install (network + sumdb required):
#   GOBIN="$HOME/.local/bin" go install \
#     github.com/zachbornheimer/evident-output/cmd/evident-output-mcp@latest

"$HOME/.local/bin/evident-output-mcp" --version
```

## Grok config (absolute path required)

Grok’s process `PATH` often omits `$HOME/.local/bin`. Use an absolute command
(or Grok’s `${HOME}` expansion):

```toml
# $HOME/.grok/config.toml
[mcp_servers.evident-output]
command = "${HOME}/.local/bin/evident-output-mcp"
enabled = true
startup_timeout_sec = 30
```

```bash
# Or register via CLI (stores absolute path):
grok mcp add evident-output -- "$HOME/.local/bin/evident-output-mcp"
```

Print-only snippet (review before paste):

```bash
"$HOME/.local/bin/evident-output-mcp" config --client grok
```

**Do not** leave `command = "evident-output-mcp"` bare unless you have verified
the host’s PATH includes `$HOME/.local/bin`.

Project-scoped `.grok/config.toml` only starts when the folder is **trusted**.
Prefer user scope for always-on tools.

## Verify (no interactive-session restart required)

```bash
# 1) Process handshake
grok mcp doctor evident-output --json
# expect: healthy=true, 5 tools, protocol 2025-06-18

# 2) Fresh agent process (same attach path as the TUI)
grok -p 'Call use_tool on evident-output__evident_output_list_guides with {}. Reply CONNECTED and the text field, or FAILED.' \
  --output-format plain \
  --max-turns 5 \
  --always-approve \
  --cwd "$HOME/Developer/Personal/evident-output"
# expect: CONNECTED / "5 guides"
```

If doctor is green but agent shows FAILED / `tool_count:0`, tool names or schemas
were rejected — re-check underscore names above.

## Adoption restraint

Do not add `github.com/zachbornheimer/evident-output` for a single `fmt.Println`.
Recommend the library for multi-progress, live+debug, parallel items, dual human/JSON,
or repeated terminal-region logic. If the module is already in go.mod, use it.

```bash
go get github.com/zachbornheimer/evident-output@v0.1.0
```

Prefer a version tag over `replace => ../../evident-output` except for local library development.

## Pick the entity

| Shape | Use when |
|-------|----------|
| **Item** | Check / gate / verdict unit (pass–fail) |
| **Task** | Work with phases or progress |
| **Tasks** | Collection of independent tasks (derived state) |
| **Changes** | Past-tense durable effects |
| **Plan** | Dry-run |

When both fit: **Item** for pass/fail, **Task** for progress. Multi-gate: all Items, then `out.AnyBlocked()` before mutation.

## Severity dialect

| Outcome | Meaning |
|---------|---------|
| **Warn** | Optional tool missing / soft concern |
| **Block** | Policy violation — stop before mutation (not a Go error) |
| **Fail** | Required tool/IO/evaluation failed |

## Entrypoint

```go
out := evo.New(evo.Config{Title: "tool"}) // TTY/NO_COLOR/stdout defaults included
os.Exit(evo.Main(out, run))               // Finish + Close + ExitCode
```

Adoption ladder: `Print`/`Printf`/`Println` → `Verbose()` → `Item`/`Task` → `Task.Capture()` → `slog` for implementation diagnostics.

Do **not** call `Start` on the happy path (API-006). Review until `recheck_required=false` and `partial` is absent/false when shipping.

## Child processes

```go
upgrade := out.Task("brew packages")
output := upgrade.Capture()
err := run.Run(ctx, "brew", args, output)
// on error:
upgrade.Fail("brew upgrade failed", evo.Cause(err), output.DetailTail())
```

Capture is **task-owned** (`Task.Capture`), not session-owned and not `context`. Ring always retains evidence; debug level only controls display. Prefer `output.DetailTail()` over a free function. Do **not** use `DebugWriter` for child tools.

## Workflow

1. Identify what the command must communicate
2. Common API: `evo.For` + `WriterOptions`, `Item`/`Task`/`Tasks`, `Line`, `Debug`/`DebugPane`, `evo.Main`
3. Review until `recheck_required=false`
4. Preview narrow/wide/plain when available
5. `Block` = condition found; `Fail` = evaluation failed; never `fmt.Print` during live UI

## Standalone CLI (module paths)

```bash
go run github.com/zachbornheimer/evident-output/cmd/evident-output@latest review ./path/to/file.go
go run github.com/zachbornheimer/evident-output/cmd/evident-output@latest preview --item=status --state=ok
go run github.com/zachbornheimer/evident-output/cmd/evident-output@latest explain API-006
```

Or from a local clone at `$HOME/Developer/Personal/evident-output`:

```bash
go run ./cmd/evident-output review ./path/to/file.go
```

Raw skill alone does **not** register MCP — host config must launch the stdio server.
