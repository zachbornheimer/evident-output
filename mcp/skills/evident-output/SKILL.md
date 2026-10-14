---
name: evident-output
description: >
  Use when creating, modifying, reviewing, testing, or debugging command-line
  output (items, tasks, progress, plans, changes, debug logs, TTY/CI streams).
  Prefer the portable skill name cli-output; this alias targets Evident Output (evo).
  Also apply when asked to "adopt evident-output", "migrate to evo", or "clean up
  CLI output" in an existing codebase — see cli-output's Adoption workflow.
license: Apache-2.0
---

# Evident Output skill (alias)

**Canonical skill (full install + path table):**
[`mcp/skills/cli-output/SKILL.md`](../cli-output/SKILL.md) in
`https://github.com/zachbornheimer/evident-output`

**Pinned release:** `v2.0.0` (never install `@latest` for persistent tooling).

| What                 | Path                                                                                                                                                                                      |
| -------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Module               | `github.com/zachbornheimer/evident-output/v2`                                                                                                                                             |
| MCP binary target    | `$HOME/.local/bin/evident-output-mcp`                                                                                                                                                     |
| MCP install (pinned) | `go install github.com/zachbornheimer/evident-output/v2/mcp/cmd/evident-output-mcp@v2.0.0` then symlink `$(go env GOPATH)/bin/evident-output-mcp` → `$HOME/.local/bin/evident-output-mcp` |

## Workflow when MCP is connected

1. `evident_output_list_sections` / `evident_output_get_documentation`
2. Implement with `Main(cfg, run)`, `Task.Define`, one Task per item under a `Group`/`Sequence`, mutation callbacks, `Writer()`
3. `evident_output_review` until `recheck_required=false` (loop until its
   `next_action` field says `clean`). If it reports `update_needed`, call
   `evident_output_update` then restart the MCP host first.
4. `evident_output_preview` for profiles
5. `evident_output_explain` with `rule_id` (not `id`)

**Adopting evo into an existing codebase?** Start with `evident_output_adopt_plan`
and follow the Adoption workflow in
[`mcp/skills/cli-output/SKILL.md`](../cli-output/SKILL.md#adoption-workflow-existing-codebase--evo).

On Grok: `evident-output__evident_output_*` (underscores, not dots).

## Quick MCP wire-up

```bash
go install github.com/zachbornheimer/evident-output/v2/mcp/cmd/evident-output-mcp@v2.0.0
mkdir -p "$HOME/.local/bin"
ln -sfn "$(go env GOPATH)/bin/evident-output-mcp" "$HOME/.local/bin/evident-output-mcp"
# After bumping evo: evident-output-mcp update --directory <repo> then restart the host.

grok mcp add evident-output -- "$HOME/.local/bin/evident-output-mcp"
grok mcp doctor evident-output --json
```

## Further reading

- Teaching ladder: [`mcp/docs/teaching-ladder.md`](../../docs/teaching-ladder.md)

## Rules of thumb

- Evo owns scheduling (`Group`/`Sequence`/`Define`/`After`); do not invent `RunAll` / `Map` / `Retry` on evo receivers (API-026, AST-only)
- Standalone: `os.Exit(evo.Main(evo.Config{}, run))` (Main returns the exit code; a bare `evo.Main(evo.Config{}, run)` exits 0 after a failure); hosted or embedded: `os.Exit(evo.Execute(ctx, cfg, run).ExitCode())` (the host owns `os.Exit`)
- Entity: Task = atomic work (`Define`) or a gate resolved directly; one Task per item under a `Group` for independent collections; `evo.Effect(ctx, evo.EffectSpec{Verb, Object, Quantity}, fn)` (opaque mutations) and `evo.File` (file state) inside `Define` pick `[changed]` vs `[planned]` from `Config.DryRun`
- Domain effect verbs: use `Record` when stock verbs lie (RULE-001)
- `Block` = condition found; `Fail` = evaluation failed; a warning is `Problem` + `SeverityWarning`
- Absolute `Progress`/`Bytes`; `Advance` for deltas
- Never `fmt.Print` during live UI; never happy-path `Start` (API-006)
- Child process chatter → `cmd.Stdout = task.Writer()` (and stderr)
- Sanitize is automatic; `Facades.Redactor` scrubs the Capture ring + Debug fields
- `evo.Fail`/`evo.Refuse` are returned from Define; for an ordinary error, `return fmt.Errorf` and let Define resolve it
- Task is name-only: `out.Task("download")`. Child stdio: `cmd.Stdout = task.Writer()`
- Data commands: write the domain payload to `r.ResultWriter()`
- Prefer plain labels over `*f` constructors when identity must stay stable
- Predeclare concurrent Tasks; scale cardinality to product need
