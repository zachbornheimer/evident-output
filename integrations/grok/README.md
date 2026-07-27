# Grok integration — Evident Output

Local **stdio** MCP only (no hosted URL). Config generation is print-only unless
you run `grok mcp add` yourself.

## 1. Install the server binary

```bash
# From a clone (dev machines):
go build -o "$HOME/.local/bin/evident-output-mcp" \
  ./cmd/evident-output-mcp

# Or:
go install github.com/zachbornheimer/evident-output/cmd/evident-output-mcp@latest

evident-output-mcp --version
```

## 2. Register with Grok

**Prefer absolute path** — Grok’s process `PATH` often omits `~/.local/bin`, which
makes bare `command = "evident-output-mcp"` fail at session attach.

```bash
evident-output-mcp config --client grok   # print TOML (review, then paste)

grok mcp add evident-output -- "$HOME/.local/bin/evident-output-mcp"
```

User-scope TOML (`~/.grok/config.toml`):

```toml
[mcp_servers.evident-output]
command = "/Users/YOU/.local/bin/evident-output-mcp"
enabled = true
startup_timeout_sec = 30
```

Do **not** rely on empty `args = []` (omit `args` instead).

### Project scope

`.grok/config.toml` in a repo is only used when the folder is **trusted**.
Untrusted project MCP → session reports failure even if user-scope doctor is green.
Trust once (`/hooks-trust` or Grok’s trusted-folders store) or stick to user scope.

## 3. Verify (no TUI restart required)

### Process handshake

```bash
grok mcp list
grok mcp doctor evident-output --json
```

Expect:

- `command found` (absolute path)
- `server started`
- `handshake OK (protocol 2025-06-18)` (also accepts `2024-11-05`, `2025-03-26`)
- `5 tools discovered`
- `healthy: true`

### Agent session (what the TUI uses)

`grok mcp doctor` can be green while an **agent** session still shows
`connection failed` or `tool_count: 0`. Probe with a **fresh headless process**:

```bash
grok -p 'Call use_tool on evident-output__evident_output_list_guides with {}. Reply CONNECTED and the text field, or FAILED.' \
  --output-format plain \
  --max-turns 5 \
  --always-approve \
  --cwd /path/to/evident-output
```

Expect: `CONNECTED` and `5 guides`.

### Debugging `tool_count: 0`

In the headless session directory under `~/.grok/sessions/…/events.jsonl`:

| Event | Meaning |
|-------|---------|
| `mcp_server_connected` + `"tool_count":5` | Good |
| `mcp_server_connected` + `"tool_count":0` | Handshake OK but tools rejected (names/schemas) |
| `mcp_server_failed` | Spawn/handshake/auth error |

**Known Grok gotcha:** tool names with **dots** (e.g. `evident_output.list_guides`)
register as **zero tools**. This server advertises **underscores**
(`evident_output_list_guides`, …). Dotted names remain accepted as call aliases.

### Transport notes

- Spec stdio: newline-delimited JSON-RPC (NDJSON).
- This server also accepts **Content-Length** frames (some SDKs).
- `serverInfo` is only `name` + `version` (strict hosts reject extra fields).
- Catalog checksum: resource `evident-output://meta/catalog-checksum`.

## 4. Tool ids in Grok

| tools/list name | use_tool name |
|-----------------|---------------|
| `evident_output_list_guides` | `evident-output__evident_output_list_guides` |
| `evident_output_get_guidance` | `evident-output__evident_output_get_guidance` |
| `evident_output_review` | `evident-output__evident_output_review` |
| `evident_output_preview` | `evident-output__evident_output_preview` |
| `evident_output_explain` | `evident-output__evident_output_explain` |

`explain` body: `{ "rule_id": "DOM-011" }`.

## 5. Skill

Portable skill: [`../../skills/cli-output/SKILL.md`](../../skills/cli-output/SKILL.md).
