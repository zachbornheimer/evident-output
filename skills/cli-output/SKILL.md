---
name: cli-output
description: >
  Use when creating, modifying, reviewing, testing, or debugging command-line
  output. Apply when a CLI prints items, tasks, multiple progress bars, plans,
  changes, warnings, errors, debug logs, tables, structured output, or next
  actions; or when stdout, stderr, TTY behavior, color, terminal width, CI
  output, exit codes, or live rendering are involved. Also apply when asked to
  "adopt evident-output", "migrate to evo", or "clean up CLI output" in an
  existing codebase — see Adoption workflow below.
license: Apache-2.0
---

# CLI Output

Portable skill for understandable CLI presentation. Prefer **Evident Output**
when available; stay useful when it is not.

**Pinned release:** `v0.4.3` (keep install commands on this pin; never `@latest`).

## Canonical locations (portable)

| What                     | Path                                                                                                       |
| ------------------------ | ---------------------------------------------------------------------------------------------------------- |
| **GitHub repo**          | `https://github.com/zachbornheimer/evident-output`                                                         |
| **Go module**            | `github.com/zachbornheimer/evident-output`                                                                 |
| **MCP package**          | `github.com/zachbornheimer/evident-output/cmd/evident-output-mcp`                                          |
| **CLI package**          | `github.com/zachbornheimer/evident-output/cmd/evident-output`                                              |
| **This skill in-repo**   | `skills/cli-output/SKILL.md`                                                                               |
| **MCP install (module)** | `GOBIN=$HOME/.local/bin go install github.com/zachbornheimer/evident-output/cmd/evident-output-mcp@v0.4.3` |

Host-specific wiring (Grok, Claude Code, Codex, …) lives under `integrations/<host>/` in the repo — not in this skill.

## Capability fallback

1. **Connected MCP** — tools below
2. **Standalone CLI** — `go run github.com/zachbornheimer/evident-output/cmd/evident-output@v0.4.3 …`
3. **This skill’s static guidance**

## MCP tool names (underscores only)

| tools/list                         | Purpose                            |
| ---------------------------------- | ---------------------------------- |
| `evident_output_list_guides`       | Catalog (token-budgeted snippets)  |
| `evident_output_get_guidance`      | Guidance snippets by id            |
| `evident_output_list_sections`     | Full docs corpus table of contents |
| `evident_output_get_documentation` | Full doc section body by id        |
| `evident_output_review`            | Go / transcript / JSON             |
| `evident_output_adopt_plan`        | Migration plan for a directory     |
| `evident_output_preview`           | Plain profiles                     |
| `evident_output_explain`           | `rule_id` (not `id`)               |

On Grok, tools are `evident-output__evident_output_*`.

## Adoption workflow (existing codebase → evo)

Trigger phrases: "adopt evident-output", "migrate to evo", "clean up CLI output".

1. **Inventory** — `evident_output_adopt_plan` with `{ "directory": "<repo or package path>" }`.
   Returns findings (file/line/pattern/suggestion) grouped by `rungs_touched`, in ladder order.
2. **Migrate rung by rung**, one rung fully before the next:
   `Init/Main → Task/Done → effects → containers → facts/warnings → confirm/dry-run`.
   Pull authoritative detail per rung with `evident_output_get_documentation` (ids
   `adoption-ladder`, `guide/common-api`, `guide/tasks`) rather than guessing spellings —
   the catalog is the single source of truth, this skill only points at it.
3. **Review the loop until clean** — after each rung's edits, call `evident_output_review`
   on the changed file(s); its `next_action` field says `clean` or tells you to re-run.
   Repeat within the rung until `recheck_required=false` and zero findings before starting
   the next rung.
4. **Verify with a pty capture** — run the CLI under a real or emulated TTY (e.g.
   `testkit.Screen` for Go call sites, or a plain terminal run for the binary) and confirm
   the live region renders as expected; a review pass with zero findings does not by itself
   prove the terminal output looks right.

Re-run `evident_output_adopt_plan` on the same directory after a rung completes — a clean
plan (no findings) is the adoption's own done-condition.

## Install library

```bash
go get github.com/zachbornheimer/evident-output@v0.4.3
```

## Philosophy (in-repo)

- `docs/philosophy/jazz-syntax.md` — one spelling per intent
- `docs/philosophy/presentation-boundary.md` — presentation ≠ execution
- `docs/philosophy/domain-vocabulary.md` — Task/Plan/Changes/Detail/Failf evidence
- `docs/guides/teaching-ladder.md` — ordinary learning order
- `docs/roadmap/implementation-basis.md` — polish-phase authority

## Adoption ladder

```text
evo.Init(Config) → Print/Printf/Println → Verbose()
→ Task / Tasks → Task.Evidence() + DetailTail
→ Plan / Changes (domain verbs via Record when needed)
→ slog via SlogHandler → os.Exit(evo.Main(run))
```

Prefer **contracts over sugar**: plain `Task` labels first; `evo.ID` when machine keys matter; `Taskf` only when the label must embed a value.

## Entrypoint

```go
evo.Init(evo.Config{Title: "tool"})
os.Exit(evo.Main(run))
```

`evo.Init(Config{Isolated: true})` + `out.Run(run)` are the advanced, hosted-instance
form of the same lifecycle — reach for them only when a tool needs an `*Output` it
doesn't install as the package-level default.

`Main` records a non-nil `run` error as Fail before Finish (no `[ready]` with exit 2).

## Child processes

```go
upgrade := out.Task("brew packages")
proof := upgrade.Evidence() // silent retention by default
if err := run.Run(ctx, "brew", args, proof); err != nil {
    return upgrade.Failf("brew upgrade failed: %w", err)
}
upgrade.Done()
```

Prefer `task.Run(cmd)` for an `*exec.Cmd` — it wires Evidence and Phase together in one call.
Opt-in display: `Evidence(evo.MirrorToDiagnostics())` or `MirrorToDebug()`.
Do **not** use `DebugWriter` for child tools (API-029).
Secrets: set `Config.Redactor` — the Evidence ring and DetailTail are redacted on retention.

## Platform contracts

| Need        | Use                                                   |
| ----------- | ----------------------------------------------------- |
| Stable key  | `out.Task("download", evo.ID("build.base"))`          |
| Namespace   | `out.Scope("registry").Task("auth", evo.ID("creds"))` |
| Domain JSON | `FormatData` + `out.ResultWriter()` (human on stderr) |

## Severity

| Outcome   | Meaning                               |
| --------- | ------------------------------------- |
| **Warn**  | Soft / optional                       |
| **Block** | Stop before mutation (not a Go error) |
| **Fail**  | Evaluation / required tool failed     |

## Review

```bash
go run github.com/zachbornheimer/evident-output/cmd/evident-output@v0.4.3 review ./path.go
```

Until `recheck_required=false`. Rules include API-006 (Start), API-026 (RunAll/Map), API-028 (Donef without %), API-029 (Capture), STREAM-003 (fmt.Print).
