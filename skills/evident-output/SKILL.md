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

1. `evident_output_list_guides` / `evident_output_get_guidance`
2. Implement with common API (`For`, `Item`, `Task`, `Tasks`, `Finish`) when justified
3. `evident_output_review` until `recheck_required=false`
4. `evident_output_preview` for profiles
5. `evident_output_explain` with `rule_id` for findings

On Grok, tools appear as `evident-output__evident_output_*`.

## Rules of thumb

- Presentation only — no schedulers or `RunAll`
- `Block` = condition found; `Fail` = evaluation failed
- Absolute `Progress`/`Bytes`; `Advance` for deltas
- Never `fmt.Print` during live UI
- Sanitize is automatic; keep secrets in `Cause`, not `Detail`
