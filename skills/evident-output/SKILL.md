---
name: evident-output
description: >
  Use when creating, modifying, reviewing, testing, or debugging command-line
  output (items, tasks, progress, plans, changes, debug logs, TTY/CI streams).
  Prefer the portable skill name cli-output; this alias targets Evident Output (evo).
license: Apache-2.0
---

# Evident Output skill (alias)

**Canonical skill (full install + path table):**  
[`skills/cli-output/SKILL.md`](../cli-output/SKILL.md) in  
`https://github.com/zachbornheimer/evident-output`

| What | Path |
|------|------|
| Module | `github.com/zachbornheimer/evident-output` |
| MCP binary target | `$HOME/.local/bin/evident-output-mcp` |
| Local clone (this Mac) | `$HOME/Developer/Personal/evident-output` |

## Workflow when MCP is connected

1. `evident_output_list_guides` / `evident_output_get_guidance`
2. Implement with `New(Config)`, `Print*`, `Item`/`Task`, `Capture`, `Main`
3. `evident_output_review` until `recheck_required=false`
4. `evident_output_preview` for profiles
5. `evident_output_explain` with `rule_id` (not `id`)

On Grok: `evident-output__evident_output_*` (underscores, not dots).

## Quick MCP wire-up

```bash
go build -o "$HOME/.local/bin/evident-output-mcp" \
  "$HOME/Developer/Personal/evident-output/cmd/evident-output-mcp"
# or: GOBIN="$HOME/.local/bin" go install github.com/zachbornheimer/evident-output/cmd/evident-output-mcp@latest

grok mcp add evident-output -- "$HOME/.local/bin/evident-output-mcp"
grok mcp doctor evident-output --json
```

## Philosophy (binding)

- [`docs/philosophy/jazz-syntax.md`](../../docs/philosophy/jazz-syntax.md)
- [`docs/philosophy/presentation-boundary.md`](../../docs/philosophy/presentation-boundary.md)
- [`docs/philosophy/domain-vocabulary.md`](../../docs/philosophy/domain-vocabulary.md)
- Teaching ladder: [`docs/guides/teaching-ladder.md`](../../docs/guides/teaching-ladder.md)
- Implementation basis: [`docs/roadmap/implementation-basis.md`](../../docs/roadmap/implementation-basis.md)

## Rules of thumb

- Presentation only — no schedulers or `RunAll` / `Map` / `Retry` (API-026, AST-only)
- Standalone: `os.Exit(evo.Main(out, run))`; hosted: Finish+Close (host owns `os.Exit`)
- Entity: Item = gate, Task = progress, Changes = did, Plan = would
- Domain effect verbs: use `Record` when stock verbs lie (RULE-001)
- `Block` = condition found; `Fail` = evaluation failed; `Warn` = optional/soft
- Absolute `Progress`/`Bytes`; `Advance` for deltas
- Never `fmt.Print` during live UI; never happy-path `Start` (API-006)
- Child process chatter → `task.Capture()` / `item.Capture()` + `DetailTail()`
- Sanitize is automatic; `Config.Redactor` scrubs Capture ring + Debug fields
- Stable machine keys: `evo.ID(...)`; plugins: `out.Scope("name")` (entities only)
- Data commands: `FormatData` + write domain payload to `out.ResultWriter()`
- Prefer plain labels over `*f` constructors when identity must stay stable
- Predeclare concurrent Tasks; scale cardinality to product need
