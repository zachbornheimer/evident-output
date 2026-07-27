# Grok integration — Evident Output

Local stdio MCP only (no hosted URL). Print-only setup; nothing writes your
config without an explicit host command.

## 1. Install the server binary

```bash
go install github.com/zachbornheimer/evident-output/cmd/evident-output-mcp@latest
# ensure $(go env GOPATH)/bin is on PATH
evident-output-mcp --version
```

## 2. Register with Grok

**Print snippet (deterministic):**

```bash
evident-output-mcp config --client grok
```

**Or use Grok’s CLI:**

```bash
grok mcp add evident-output -- evident-output-mcp
# project-scoped (commit .grok/config.toml):
# grok mcp add --scope project evident-output -- evident-output-mcp
```

**Or paste into `~/.grok/config.toml` / project `.grok/config.toml`:**

```toml
[mcp_servers.evident-output]
command = "evident-output-mcp"
args = []
enabled = true
startup_timeout_sec = 30
```

## 3. Verify

```bash
grok mcp list
# User-scope binary (recommended) — healthy from any cwd:
grok mcp doctor evident-output

# Project-scoped entries may report "folder untrusted" until Grok trusts the repo.
# Prefer user scope for always-on tools:
#   grok mcp add evident-output -- "$HOME/.local/bin/evident-output-mcp"
```

Expect (example, verified 2026-07-27):

- `command found`
- `server started`
- `handshake OK (protocol 2025-06-18)`
- `5 tools discovered`

**Notes:**

- An already-running Grok session may not load new MCP tools until you start a **new** session.
- This chat session may still lack `evident_output.*` until restart even when `grok mcp doctor` is green.

## 4. Skill

Copy or symlink [`../../skills/cli-output/SKILL.md`](../../skills/cli-output/SKILL.md)
into the host skill path if not already discovered via the repo.
