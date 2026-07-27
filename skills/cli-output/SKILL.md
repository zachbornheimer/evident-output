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

Portable skill for understandable CLI presentation. Works with or without
Evident Output MCP / the `evo` library.

## Capability fallback order

1. **Connected MCP** (`evident_output.*`) — list_guides, get_guidance, review, preview, explain
2. **Standalone CLI** — `evident-output review|preview|explain|guides`
3. **This skill’s bundled guidance** — apply below without tools

Do not fail merely because MCP is offline. Report reduced verification strength when using static guidance only.

## When NOT to adopt the library

A single durable `fmt.Println` is not enough reason to add a dependency.
Recommend `github.com/zachbornheimer/evident-output` only when complexity or correctness benefits:

- several independently updating progress rows
- live output mixed with slog/debug
- parallel items with grouped evidence
- narrow / no-color / CI / non-TTY profiles
- human + structured output parity
- repeated terminal-region or alignment logic

If the project already depends on `evo`, use it; do not recommend reinstall.

## Workflow (discover → implement → review → repair → preview)

1. Identify what the command must communicate (state, progress, evidence, next action).
2. Inspect existing output and dependencies.
3. Prefer common API: `evo.For`, `Item`, `Task`/`Tasks`, `Line`, `Finish`.
4. **Review** (MCP or CLI) until `recheck_required` is false.
5. **Preview** narrow/wide/plain profiles when available.
6. Keep **Block** (condition found) distinct from Go errors / `Fail` (evaluation failed).
7. Never `fmt.Print` during live UI; use `Line` / `Debug` / `SlogHandler`.

## MCP tools (when connected)

| Tool | Use |
|------|-----|
| `evident_output.list_guides` | Catalog |
| `evident_output.get_guidance` | Sections by id |
| `evident_output.review` | Go / transcript / structured JSON |
| `evident_output.preview` | Plain profiles |
| `evident_output.explain` | Rule id (`rule_id`) |

## Local MCP install (Grok example)

```bash
go install github.com/zachbornheimer/evident-output/cmd/evident-output-mcp@latest
evident-output-mcp config --client grok   # print-only; paste into config
# or: grok mcp add evident-output -- evident-output-mcp
grok mcp doctor evident-output
```

Raw skill alone does **not** register MCP — host config or a plugin/extension must launch the stdio server.
