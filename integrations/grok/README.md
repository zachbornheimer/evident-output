# Grok integration — Evident Output

Local **stdio** MCP only (no hosted URL).

## Canonical paths

| What | Path |
|------|------|
| GitHub | `https://github.com/zachbornheimer/evident-output` |
| Module / MCP cmd | `github.com/zachbornheimer/evident-output/cmd/evident-output-mcp` |
| Local clone (this Mac) | `$HOME/Developer/Personal/evident-output` |
| Binary install target | `$HOME/.local/bin/evident-output-mcp` |
| Grok user config | `$HOME/.grok/config.toml` |
| Skill | `skills/cli-output/SKILL.md` in the repo |

## 1. Install the server binary

```bash
mkdir -p "$HOME/.local/bin"

# Preferred when the repo is cloned at the path above:
go build -o "$HOME/.local/bin/evident-output-mcp" \
  "$HOME/Developer/Personal/evident-output/cmd/evident-output-mcp"

# From an arbitrary clone:
#   git clone https://github.com/zachbornheimer/evident-output.git
#   cd evident-output && go build -o "$HOME/.local/bin/evident-output-mcp" ./cmd/evident-output-mcp

# Module install (network + sumdb):
#   GOBIN="$HOME/.local/bin" go install \
#     github.com/zachbornheimer/evident-output/cmd/evident-output-mcp@latest

"$HOME/.local/bin/evident-output-mcp" --version
```

## 2. Register with Grok (absolute path)

Grok’s process `PATH` often omits `$HOME/.local/bin`. **Do not** use a bare
`command = "evident-output-mcp"` unless you have verified PATH.

```bash
grok mcp add evident-output -- "$HOME/.local/bin/evident-output-mcp"
# print-only TOML (replace YOU only if the generator still shows a placeholder):
"$HOME/.local/bin/evident-output-mcp" config --client grok
```

User-scope TOML (`$HOME/.grok/config.toml`) — Grok expands `${HOME}`:

```toml
[mcp_servers.evident-output]
command = "${HOME}/.local/bin/evident-output-mcp"
enabled = true
startup_timeout_sec = 30
```

Omit empty `args = []`.

### Project scope

Repo `.grok/config.toml` only starts when the folder is **trusted**. Prefer user
scope for always-on tools. Untrusted project MCP can make sessions report
failure even when user-scope `doctor` is green.

## 3. Verify without restarting the TUI

```bash
# Process handshake
grok mcp doctor evident-output --json
# expect: healthy=true, 5 tools, protocol 2025-06-18

# Fresh agent process (same attach path as the TUI)
grok -p 'Call use_tool on evident-output__evident_output_list_guides with {}. Reply CONNECTED and the text field, or FAILED.' \
  --output-format plain \
  --max-turns 5 \
  --always-approve \
  --cwd "$HOME/Developer/Personal/evident-output"
# expect: CONNECTED / "5 guides"
```

### Debugging

In `~/.grok/sessions/…/events.jsonl`:

| Event | Meaning |
|-------|---------|
| `mcp_server_connected` + `"tool_count":5` | Good |
| `mcp_server_connected` + `"tool_count":0` | Tools rejected (use underscore names) |
| `mcp_server_failed` | Spawn/handshake error (path/PATH) |

**Grok gotcha:** dotted tool names (`evident_output.list_guides`) → `tool_count: 0`.
This server advertises **underscores** (`evident_output_list_guides`, …).

## 4. Tool ids in Grok

| tools/list | use_tool |
|------------|----------|
| `evident_output_list_guides` | `evident-output__evident_output_list_guides` |
| `evident_output_get_guidance` | `evident-output__evident_output_get_guidance` |
| `evident_output_review` | `evident-output__evident_output_review` |
| `evident_output_preview` | `evident-output__evident_output_preview` |
| `evident_output_explain` | `evident-output__evident_output_explain` |

`explain` body: `{ "rule_id": "DOM-011" }`.

## 5. Skill

[`../../skills/cli-output/SKILL.md`](../../skills/cli-output/SKILL.md) — path table and install steps are the source of truth for agents.
