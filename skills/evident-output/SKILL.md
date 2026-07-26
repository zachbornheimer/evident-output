---
name: evident-output
description: Use Evident Output (evo) for CLI presentation — items, tasks, progress, plain/JSON, live regions.
---

# Evident Output skill

## When to use

Building or reviewing Go CLI presentation with `github.com/zachbornheimer/evident-output`.

## Workflow

1. `evident_output.list_guides` / `get_guidance` for the task
2. Implement with common API (`For`, `Item`, `Task`, `Tasks`, `Finish`)
3. `evident_output.review` on the Go source
4. Repair findings; re-run review until `recheck_required=false`
5. `evident_output.preview` for narrow/wide plain profiles

## Rules of thumb

- Presentation only — no schedulers or `RunAll`
- `Block` = condition found; `Fail` = evaluation failed
- Absolute `Progress`/`Bytes`; `Advance` for deltas
- Never `fmt.Print` during live UI
- Sanitize is automatic; keep secrets in `Cause`, not `Detail`
