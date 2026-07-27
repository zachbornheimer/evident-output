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
grok mcp doctor evident-output
```

Expect: server starts, `initialize` negotiates a supported protocol
(`2024-11-05` / `2025-03-26` / `2025-06-18`), tools include
`evident_output.list_guides`, `get_guidance`, `review`, `preview`, `explain`.

**Note:** An already-running Grok session may not pick up new MCP servers
until you start a new session.

## 4. Skill

Copy or symlink [`../../skills/cli-output/SKILL.md`](../../skills/cli-output/SKILL.md)
into the host skill path if not already discovered via the repo.
