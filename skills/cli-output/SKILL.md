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

Portable skill for understandable CLI presentation. Prefer Evident Output MCP
when connected; otherwise use the standalone CLI or this guidance.

## Capability fallback

1. **MCP** — tools below (Grok: `evident-output__evident_output_*`)
2. **CLI** — `evident-output review|preview|explain`
3. **Static skill guidance** — apply without tools

Do not fail merely because MCP is offline. Report reduced verification strength when using static guidance only.

## Adoption restraint

Do not add `github.com/zachbornheimer/evident-output` for a single `fmt.Println`.
Recommend the library for multi-progress, live+debug, parallel items, dual human/JSON,
or repeated terminal-region logic. If the module is already in go.mod, use it.

## Workflow

1. Identify what the command must communicate
2. Common API: `evo.For`, `Item`, `Task`/`Tasks`, `Line`, `Debug`/`DebugPane`, `Finish`
3. Review until `recheck_required` is false
4. Preview narrow/wide/plain when available
5. `Block` = condition found; `Fail` = evaluation failed; never `fmt.Print` during live UI

## MCP tools (when connected)

Advertised names use **underscores** (not dots). Grok qualifies as `server__tool`.

| tools/list | Grok use_tool | Use |
|------------|---------------|-----|
| `evident_output_list_guides` | `evident-output__evident_output_list_guides` | Catalog |
| `evident_output_get_guidance` | `evident-output__evident_output_get_guidance` | Sections by id |
| `evident_output_review` | `evident-output__evident_output_review` | Go / transcript / JSON |
| `evident_output_preview` | `evident-output__evident_output_preview` | Plain profiles |
| `evident_output_explain` | `evident-output__evident_output_explain` | `rule_id` |

## Local MCP (Grok)

```bash
go build -o "$HOME/.local/bin/evident-output-mcp" \
  ./cmd/evident-output-mcp   # or go install …@latest

# Absolute path in ~/.grok/config.toml (PATH is often incomplete for GUI hosts)
grok mcp add evident-output -- "$HOME/.local/bin/evident-output-mcp"

# Verify without restarting an interactive session:
grok mcp doctor evident-output --json
grok -p 'Call use_tool on evident-output__evident_output_list_guides. Reply CONNECTED or FAILED.' \
  --output-format plain --max-turns 5 --always-approve
```

Raw skill alone does **not** register MCP — host config must launch the stdio server.
