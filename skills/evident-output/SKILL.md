---
name: evident-output
description: >
  Use when creating, modifying, reviewing, testing, or debugging command-line
  output (items, tasks, progress, plans, changes, debug logs, TTY/CI streams).
  Prefer the portable skill name cli-output; this alias targets Evident Output (evo).
license: Apache-2.0
---

# Evident Output skill (alias)

Canonical portable skill: [`../cli-output/SKILL.md`](../cli-output/SKILL.md).

## Workflow when MCP is connected

1. `evident_output.list_guides` / `get_guidance`
2. Implement with common API (`For`, `Item`, `Task`, `Tasks`, `Finish`) when justified
3. `evident_output.review` until `recheck_required=false`
4. `evident_output.preview` for profiles
5. `evident_output.explain` with `rule_id` for findings

## Rules of thumb

- Presentation only — no schedulers or `RunAll`
- `Block` = condition found; `Fail` = evaluation failed
- Absolute `Progress`/`Bytes`; `Advance` for deltas
- Never `fmt.Print` during live UI
- Sanitize is automatic; keep secrets in `Cause`, not `Detail`
