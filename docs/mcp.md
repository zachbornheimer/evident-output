# MCP (stdio)

The companion server `evident-output-mcp` is a **local stdio** MCP process (no hosted URL).
Stdin/stdout are JSON-RPC only; logs go to **stderr**. Transport supports **NDJSON** (MCP
spec) and **Content-Length** framing (some client SDKs).

**Advertised tool names** use underscores (Grok rejects dotted tool names and then
registers `tool_count: 0`). Dotted aliases still work on `tools/call`.

| Tool name (tools/list)             | Grok `use_tool` id                                 | Purpose                                    |
| ---------------------------------- | -------------------------------------------------- | ------------------------------------------ |
| `evident_output_list_guides`       | `evident-output__evident_output_list_guides`       | Guidance catalog (token-budgeted snippets) |
| `evident_output_get_guidance`      | `evident-output__evident_output_get_guidance`      | Guidance snippets by id                    |
| `evident_output_list_sections`     | `evident-output__evident_output_list_sections`     | Full docs corpus table of contents         |
| `evident_output_get_documentation` | `evident-output__evident_output_get_documentation` | Full doc section body by id                |
| `evident_output_review`            | `evident-output__evident_output_review`            | Go / transcript / JSON review              |
| `evident_output_adopt_plan`        | `evident-output__evident_output_adopt_plan`        | Migration plan for a non-evo directory     |
| `evident_output_preview`           | `evident-output__evident_output_preview`           | Plain profile previews                     |
| `evident_output_explain`           | `evident-output__evident_output_explain`           | Rule id (`rule_id`)                        |

`explain` arguments: `{ "rule_id": "DOM-011" }` (not `id`).

`evident_output_list_sections` / `evident_output_get_documentation` serve the full docs
corpus (this file, `reference.md`, `development.md`, the adoption ladder, and every
`evident_output_list_guides` entry as `guide/<id>`) so an agent never needs local file
access to read authoritative docs — call `list_sections` for the table of contents, then
`get_documentation` with the `id`s you need.

`evident_output_adopt_plan` takes `{ "directory": "..." }`, statically inventories
`fmt.Print*`/`log.*`/`os.Stdout` writes and manual spinner/progress-bar imports under it,
and returns findings keyed to the adoption ladder rung each belongs on — see the
adoption workflow in [`../skills/cli-output/SKILL.md`](../skills/cli-output/SKILL.md).

Call `evident_output_review` again after applying its suggested fixes — the response's
`next_action` field says `clean` at zero findings or tells you to re-run; loop until clean.

## Install the binary (pinned)

```bash
GOBIN="$HOME/.local/bin" go install github.com/zachbornheimer/evident-output/cmd/evident-output-mcp@v0.4.3
"$HOME/.local/bin/evident-output-mcp" --version
```

The pinned tag is kept in sync by `mise run sync-release-pins`. A
client-specific copy-paste block lives under
[`../integrations/`](../integrations/) — Claude Code, Codex, Grok, Gemini, opencode.

Host configs must use an **absolute** command (or `${HOME}/…` where the host expands
it). Bare `evident-output-mcp` fails when the agent process PATH omits `~/.local/bin`.

## Verify without restarting an existing TUI session

```bash
# Process-level handshake
grok mcp doctor evident-output --json
# expect: healthy=true, "8 tools discovered", protocol 2025-06-18

# Fresh agent process (same attach path as the TUI); use any trusted cwd:
grok -p 'Call use_tool on evident-output__evident_output_list_guides with {}. Reply CONNECTED and the text field, or FAILED.' \
  --output-format plain \
  --max-turns 5 \
  --always-approve
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

## Host config snippets (print-only)

```bash
"$HOME/.local/bin/evident-output-mcp" config --client grok
# also: claude-code|codex|gemini|opencode  — uses ${HOME}/.local/bin/…
```

Integrations: [`../integrations/`](../integrations/) · skill: [`../skills/cli-output/SKILL.md`](../skills/cli-output/SKILL.md)

### Grok (xAI TUI / Build)

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

See [`../integrations/grok/README.md`](../integrations/grok/README.md).

### Claude Code / Cursor / Codex

```bash
"$HOME/.local/bin/evident-output-mcp" config --client claude-code
"$HOME/.local/bin/evident-output-mcp" config --client codex
```

Review kinds for `evident_output_review`: `go` (default), `transcript`, `json` / `structured`.
