# Evident Output

Go presentation library for CLI **state, progress, evidence, changes, plans, and conclusions**.

Application code owns execution. Package `evo` owns presentation only.

## Quick start

```go
import (
    "os"
    evo "github.com/zachbornheimer/evident-output"
)

func main() {
    evo.Init(evo.Config{Title: "bpp-csharp"}) // first statement — arms first paint before any I/O
    os.Exit(evo.Main(run))
}

func run() error {
    // Start as casually as fmt — then promote to structure when useful.
    evo.Println("Reading configuration")
    evo.Printf("Found %d packages\n", 18)

    evo.Task("working tree").Done()
    evo.Task("branches").Block(
        "local-only branch",
        evo.Detail("commit or stash before continuing"),
    )

    evo.Task("cleanup").Delete(2, "stale local branch") // singular object, ledger renders "2 stale local branches"; [changed]/[planned] picked from Config.DryRun; no Done needed — a recorded effect auto-resolves
    for pkg := range evo.Task("install").Each(packages) {
        install(pkg)
    }
    return nil // Block is a presentation outcome, not a Go error
}
```

```bash
go get github.com/zachbornheimer/evident-output@v0.2.16
```

Requires **Go 1.25+**. License: **Apache-2.0**.

Design philosophy and polish-phase basis: [`docs/roadmap/implementation-basis.md`](docs/roadmap/implementation-basis.md), [`docs/philosophy/`](docs/philosophy/).

**Construction:** `evo.Init(Config{…})` is the sole constructor — the package-level default instance (front door) by default; `Config.Isolated: true` returns an independent hosted instance instead — TTY, `NO_COLOR`, stdout/stderr defaults included. Advanced: `Config.Options: []Option{Title(...), …}` for exact writer/terminal/clock wiring.
**Config honesty:** `VisibilityDelay: evo.Delay(0)` is immediate (nil = default 80ms). `Debug.Level: evo.LevelDebug` selects the journal threshold — `evo.LogLevel`, a distinct type from stdlib `slog.Level` (`LevelUnset` → Info).
**Lifecycle:** `os.Exit(evo.Main(run))` (default instance, `run func() error`) or `os.Exit(out.Run(run))` (hosted, `Config.Isolated: true`, `run func(*Output) error`) seals Finish + Close + exit code; a non-nil `run` error is recorded as Fail only when nothing already failed.
**Messages:** one human instrument — `Print` / `Printf` / `Println` + `Verbose()`. Infrastructure logs: `slog.New(out.SlogHandler())` (level from `Config.Debug.Level` only). Semantic state: `Task`.
**Mutations:** `Task.Add/Delete/Create/Update/Remove/Write/Push/Record/RecordName` pick `[planned]` vs `[changed]` from `Config.DryRun` — one spelling, never a call-site tense flip.
**Loops and taxonomy:** `Task.Each(items []string)` / `EachN(len(items))` (any other slice type) own absolute progress; `Task.Skipped(reason, name)` / `Task.Kept(reason, name)` own the counted, summed skip/keep partition.
**Confirm:** `evo.Confirm(question, …)` owns the whole ask-decide-resolve gate — `Done` / `⊘ declined` / `⊘ blocked by policy`, never a Go error. `question` is literal text, not a printf format — Confirm is the one entity-text spelling that takes no variadic fmt args (every other one — Task/Done/Warn/Phase/Skip/Group/Reason — is printf-variadic), so build the string yourself (`fmt.Sprintf`) before calling.
**Capture:** `Task.Evidence` (work or tool-backed gate); silent by default; pending fragments in `DetailTail`; `Config.Redactor` before retention. `cmd.Stdout = task.PhaseWriter()` turns a talkative child's last line into the live Phase; `out.Suspend(fn)` hands the tty to a child that paints its own UI.
**Platform:** `evo.ID` + narrow `Scope` (Task/Tasks only — not a sandbox); `ResultWriter()` under `FormatData`.

## Pick the entity

| Shape     | Use when                                                                                                                                                                                                    |
| --------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Task**  | Everything — a check/gate resolved directly (`Done`/`Warn`/`Block`/`Fail`/`Skip`, no `Phase`/`Progress`) renders as a fact row; work with phases, progress, or mutation verbs shows a spinner while running |
| **Tasks** | Collection of independent tasks (state is **derived**)                                                                                                                                                      |

Multi-gate: resolve every Task, then `if out.AnyBlockedSoFar() { return nil }` before mutation; `Main` maps `ExitCode`.

**Advanced (tooling call sites):** `Plan` / `Changes` are the instance-API primitives Task's mutation verbs (`Delete`/`Create`/`Update`/…) are built on — reach for them directly only when a tool needs the would/did split without a Task.

## Severity dialect

| Outcome   | Meaning                                                                       |
| --------- | ----------------------------------------------------------------------------- |
| **Warn**  | Soft concern or **optional** tool missing; command may continue               |
| **Block** | Policy / precondition failed; **stop before mutation** (evaluation succeeded) |
| **Fail**  | Evaluation failed or **required** tool/IO failed                              |

`Block` ≠ Go `error`. After Block, return nil from `run` and let `Main` exit `1`.

## Conclusion band → exit code

The trailing `[state]` band and the process exit code always agree — never read
one without checking the other. `· partial` is a modifier on the state, not a
state of its own: an abandoned loop or a forgotten terminal verb on an
otherwise clean finish adds `· partial` to whatever state the run already
concluded, without changing its exit code.

| Band                           | Exit code | Meaning                                                                  |
| ------------------------------ | --------- | ------------------------------------------------------------------------ |
| `[changed]`                    | `0`       | A mutation verb (`Delete`/`Create`/…) recorded outside `DryRun`          |
| `[planned]`                    | `0`       | A mutation verb recorded under `Config.DryRun` (would, not did)          |
| `[ready]`                      | `0`       | Every task resolved `Done`; no mutation verb recorded                    |
| `[unchanged]`                  | `0`       | Every task resolved `Done.Unchanged`; nothing needed to change           |
| `[blocked]`                    | `1`       | At least one `Block`, and nothing `Fail`ed                               |
| `[failed]`                     | `2`       | At least one `Fail`, or a caller-supplied misuse                         |
| `[cancelled]`                  | `130`     | `Cancel` or an interrupt ended the run early                             |
| any of the above + `· partial` | unchanged | The run also left an unresolved task — same exit code as the state above |

## Child processes / tool-backed gates

Evidence belongs to the **entity** (a `Task`, whether it ran or was resolved as a
fact-check gate), not the whole session — and not `context`.
For an `*exec.Cmd`, prefer `Task.Run` (below); reach for `Evidence` directly only when the
caller already owns stdout/stderr plumbing:

```go
upgrade := out.Task("brew packages")
proof := upgrade.Evidence() // always retains a bounded ring; debug only controls display

if err := run.Run(ctx, "brew", []string{"upgrade", "--formula"}, proof); err != nil {
    return upgrade.Failf("brew upgrade failed: %w", err)
}
upgrade.Done()
```

Tool-backed **condition** (a `Task` resolved directly, no `Phase`/`Progress`):

```go
docker := out.Task("docker daemon")
proof := docker.Evidence()
if err := runDockerInfo(proof); err != nil {
    docker.Failf("could not inspect the daemon: %w", err)
} else {
    docker.Done()
}
```

- **Ownership:** `Task.Evidence` associates evidence with that entity.
- **Silent by default:** ring always retains; no Diagnostics/Debug mirror unless `MirrorToDiagnostics()` / `MirrorToDebug()`.
- **Redaction:** `Config.Redactor` (or `evo.Redact`) applies before ring retention.
- **Detail:** `DetailTail()` is a `ProblemOption`; separate `Stdout()`/`Stderr()` buffers. `Failf`'s
  trailing `%w` also renders a summary/evidence split for the wrapped error itself.
- **Defaults:** last 200 lines / 256KiB via `KeepLastLines` / `MaxEvidenceBytes`.
- **Session `out.Capture`:** advanced only — prefer entity-owned capture.

## Platform adapters (contracts, not sugar)

Keep the core vocabulary small. Scale via **Config**, **schema keys**, and **stream contracts**:

| Need                         | Contract                                                                                               |
| ---------------------------- | ------------------------------------------------------------------------------------------------------ |
| Stable machine identity      | `out.Task("label", evo.ID("gate.tree"))` — keys appear in Snapshot/JSON                                |
| Plugin / subsystem namespace | `out.Scope("registry").Task("pull", evo.ID("image"))` → key `registry.image` (IDs only; not isolation) |
| Domain payload purity        | `Format: FormatData` + `json.NewEncoder(out.ResultWriter())` (stdout); human on stderr                 |
| Secret scrubbing             | `Config.Redactor` or `evo.Redact(r)` — Debug fields + Capture ring                                     |
| Host-owned rendering         | `FormatExternal` + `out.Snapshot()` (no inline stream)                                                 |

Avoid inventing parallel APIs (`RunAll`, framework-specific facades in core). Prefer one `Config` field or `EntityOption` over a new top-level type.

## Status

**Release:** **v0.2.16** — pin **maintenance class** closed: `PublishedRelease` + auto-walk drift gate + `sync-release-pins` (no ad-hoc skill/README pin edits). Portable install forbids `@latest` and personal clone paths.
**Architecture spec:** [v0.5](docs/architecture/EVIDENT_OUTPUT_ARCHITECTURE_SPEC_v0.5.md) (design candidate).
**Implemented surface:** ordinary ladder through Plan/Changes/Capture/slog/ResultWriter; interactive VT; hardened MCP; polish-phase docs under `docs/`. External/manual items remain waived (Windows ConPTY / tmux / SSH RC, a11y contrast / screen-reader, host RC matrices and a11y manual reviews).

| Ready now                                                                                                                   | External / manual only                |
| --------------------------------------------------------------------------------------------------------------------------- | ------------------------------------- |
| Task, Tasks, Changes, Plan, Print                                                                                           | Windows ConPTY RC (PORT-003)          |
| Conclusion + exit codes + Cancel cleanup                                                                                    | tmux RC (PORT-004)                    |
| Plain, JSON (§25.1), JSONL (§25.2)                                                                                          | SSH RC (PORT-005)                     |
| Interactive live region (`testkit.Screen`)                                                                                  | Light/dark contrast review (A11Y-006) |
| `SlogHandler`, `DebugWriter`, `Suspend`, `Snapshots()`, `MaxEntities`, `MaxEvents`, `AlsoWrite`                             | Screen-reader review (A11Y-007)       |
| Appendix H.1–H.22 + agent harness + multi-file GoPackage review                                                             | —                                     |
| ANSI driver + width/CJK + OSC strip + s390x cross-compile                                                                   | —                                     |
| CLI: `review` / `preview` / `explain` (real JSON)                                                                           | —                                     |
| MCP: lifecycle, protocol negotiate, unknown-field reject, panic contain, token budget, remote-path reject, catalog checksum | —                                     |
| Framework adapter examples (urfave/Kong shapes, no core deps)                                                               | —                                     |

## Vocabulary

| Type               | Meaning                                                                                                     |
| ------------------ | ----------------------------------------------------------------------------------------------------------- |
| `Task`             | One operation — a named condition resolved directly (Done/Warn/Block/Fail/Skip) or work with phase/progress |
| `Tasks`            | Collection of independent tasks (state is **derived**)                                                      |
| `Problem`          | Structured evidence for warn / block / fail                                                                 |
| `Changes` / `Plan` | Effects that happened vs would happen                                                                       |
| `Conclusion`       | Headline + `Changed` / `Partial` / `Cancelled` + exit code                                                  |
| `Main`             | Finish + Close + process exit code for CLI entrypoints                                                      |

Do **not** put schedulers, `RunAll`, retries, or shell execution in this library. Review rule **API-026** flags those helpers only on evo receivers (AST), not `strings.Map`.

## Develop

```bash
mise run setup    # go mod download
mise run test     # unit + roast
mise run test-race
mise run conformance
mise run traceability   # all §31 IDs present
mise run ci             # lint + test + scan + conformance + traceability
```

Trunk is configured **daemonless** (`--monitor=false`). Prefer `mise` over raw tools.

### Conformance (roast)

`conformance/` is the executable specification (Raku/`roast` model):

- `TRACEABILITY.md` — all **272** §31 IDs dispositioned (**267 pass**, **5 waived** with reason + owner for external/manual only; **0 untested**)
- `schema/scenario.v1.json` — declarative scenario dialect
- `scenarios/*.json` + Go Appendix H tests (`appendix_h_test.go`)

Architecture source (current design candidate): [`docs/architecture/EVIDENT_OUTPUT_ARCHITECTURE_SPEC_v0.5.md`](docs/architecture/EVIDENT_OUTPUT_ARCHITECTURE_SPEC_v0.5.md).
Prior implemented baseline: [`docs/architecture/EVIDENT_OUTPUT_ARCHITECTURE_SPEC_v0.3.md`](docs/architecture/EVIDENT_OUTPUT_ARCHITECTURE_SPEC_v0.3.md).

Completeness vs §31 (v0.3 matrix): [`docs/architecture/COMPLETENESS_MATRIX.md`](docs/architecture/COMPLETENESS_MATRIX.md) (**267 pass / 5 waived**).

### Examples (adoption ladder)

```text
examples/print/              Print, Printf, Println
examples/verbose/            visibility gating (--verbose)
examples/repo-status/        Tasks, Problems, actions
examples/install-pipeline/   Tasks + Capture
examples/migrate/            Plan versus Changes
examples/doctor/             severity dialect + WriteJSON
examples/data-command/       machine stdout / human stderr (ResultWriter)
examples/scope-plugin/       Scope + ID for plugin namespaces
examples/live-progress/      ordinary multi-progress
examples/debug-history/      slog durable debug
examples/debug-pane/         rolling slog viewport
examples/terminal-driver/    advanced custom TerminalDriver
```

```bash
mise run examples          # non-interactive batch
go run ./examples/print/
go run ./examples/verbose/ --verbose
```

### CLI

```bash
go run ./cmd/evident-output review path/to/file.go   # JSON findings (exit 1 if recheck_required)
go run ./cmd/evident-output preview --item=status --state=ok
go run ./cmd/evident-output explain API-006
go run ./cmd/evident-output version
```

### MCP (stdio)

The companion server `evident-output-mcp` is a **local stdio** MCP process (no hosted URL).
Stdin/stdout are JSON-RPC only; logs go to **stderr**. Transport supports **NDJSON** (MCP
spec) and **Content-Length** framing (some client SDKs).

**Advertised tool names** use underscores (Grok rejects dotted tool names and then
registers `tool_count: 0`). Dotted aliases still work on `tools/call`.

| Tool name (tools/list)        | Grok `use_tool` id                            | Purpose                       |
| ----------------------------- | --------------------------------------------- | ----------------------------- |
| `evident_output_list_guides`  | `evident-output__evident_output_list_guides`  | Guidance catalog              |
| `evident_output_get_guidance` | `evident-output__evident_output_get_guidance` | Sections by id                |
| `evident_output_review`       | `evident-output__evident_output_review`       | Go / transcript / JSON review |
| `evident_output_preview`      | `evident-output__evident_output_preview`      | Plain profile previews        |
| `evident_output_explain`      | `evident-output__evident_output_explain`      | Rule id (`rule_id`)           |

`explain` arguments: `{ "rule_id": "DOM-011" }` (not `id`).

#### Install the binary (pinned)

```bash
mkdir -p "$HOME/.local/bin"

# Module install (network + sumdb) — pin a release tag, never @latest:
GOBIN="$HOME/.local/bin" go install \
  github.com/zachbornheimer/evident-output/cmd/evident-output-mcp@v0.2.16

# Or from a local clone of this repo:
#   git clone https://github.com/zachbornheimer/evident-output.git
#   cd evident-output && go build -o "$HOME/.local/bin/evident-output-mcp" ./cmd/evident-output-mcp

"$HOME/.local/bin/evident-output-mcp" --version
```

Host configs must use an **absolute** command (or `${HOME}/…` where the host expands
it). Bare `evident-output-mcp` fails when the agent process PATH omits `~/.local/bin`.

#### Verify without restarting an existing TUI session

```bash
# Process-level handshake
grok mcp doctor evident-output --json
# expect: healthy=true, "5 tools discovered", protocol 2025-06-18

# Fresh agent process (same attach path as the TUI); use any trusted cwd:
grok -p 'Call use_tool on evident-output__evident_output_list_guides with {}. Reply CONNECTED and the text field, or FAILED.' \
  --output-format plain \
  --max-turns 5 \
  --always-approve
# expect: CONNECTED / "5 guides"
```

If doctor is green but headless says FAILED, check session `events.jsonl` for
`mcp_server_connected` with `"tool_count":0` (tool names/schemas rejected) vs
`mcp_server_failed` (spawn/handshake).

NDJSON smoke (no Grok required):

```bash
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"manual","version":"0"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' \
  | "$HOME/.local/bin/evident-output-mcp" 2>/dev/null
```

#### Host config snippets (print-only)

```bash
"$HOME/.local/bin/evident-output-mcp" config --client grok
# also: claude-code|codex|gemini|opencode  — uses ${HOME}/.local/bin/…
```

Integrations: [`integrations/`](integrations/) · skill: [`skills/cli-output/SKILL.md`](skills/cli-output/SKILL.md)

#### Grok (xAI TUI / Build)

```toml
# $HOME/.grok/config.toml  (${HOME} expanded by Grok)
[mcp_servers.evident-output]
command = "${HOME}/.local/bin/evident-output-mcp"
enabled = true
startup_timeout_sec = 30
```

```bash
grok mcp add evident-output -- "$HOME/.local/bin/evident-output-mcp"
grok mcp list
grok mcp doctor evident-output --json
```

**Project scope** only starts when the folder is **trusted**. Prefer user scope for
always-on tools.

See [`integrations/grok/README.md`](integrations/grok/README.md).

#### Claude Code / Cursor / Codex

```bash
"$HOME/.local/bin/evident-output-mcp" config --client claude-code
"$HOME/.local/bin/evident-output-mcp" config --client codex
```

Review kinds for `evident_output_review`: `go` (default), `transcript`, `json` / `structured`.

### Examples

Small real programs (flags, help, exit codes) — not snippets. Copy a whole folder as a starting shape.

| Example            | Pattern                                                               |
| ------------------ | --------------------------------------------------------------------- |
| `repo-status`      | Parallel **Tasks** (done / blocked / warn), conclusion exit code      |
| `install-pipeline` | **Tasks** collection with Progress/Bytes/Fail (final report)          |
| `migrate`          | **Plan** dry-run vs **Changes** apply (`--apply`)                     |
| `doctor`           | Mixed doctor items; `--json` snapshot on stdout                       |
| `data-command`     | Data command: JSON **stdout**, human report **stderr**                |
| `live-progress`    | **Live multi-progress**: bars + indeterminate phases (ANSI on stderr) |
| `debug-history`    | **DebugHistory**: durable `HH:MM:SS.mmm [DEBUG] …` above live/items   |
| `debug-pane`       | **DebugPane**: rolling slog pane; `--fail` keeps diagnostics tail     |

```bash
mise run examples                          # all, back-to-back with headers
go run ./examples/repo-status/ --name my-app
go run ./examples/install-pipeline/
go run ./examples/migrate/                 # dry-run plan
go run ./examples/migrate/ --apply
go run ./examples/doctor/ --json | jq .conclusion
go run ./examples/data-command/ 2>/dev/null | jq .
go run ./examples/live-progress/              # in-place ANSI live region (real TTY)
go run ./examples/live-progress/ --frames     # numbered frames you can scroll
go run ./examples/debug-history/              # history-mode debug interleave
go run ./examples/debug-pane/                 # pane removed on success
go run ./examples/debug-pane/ --fail          # failure preserves diagnostics tail

# mise run examples: uses live ANSI when stderr is a TTY; --frames otherwise.
# EVO_EXAMPLES_FRAMES=1 mise run examples   # force scrubable frames in the batch
```

### Machine output

```go
snap := out.Snapshot()
plain, _ := evo.RenderPlain(snap, evo.PlainOptions{Width: 80})
jsonBytes, _ := evo.EncodeJSON(snap)
jsonl, _ := evo.EncodeJSONL(out.Events())
```

Schemas: `schema/output.v1.json`, `schema/event.v1.json`.

### Production ANSI driver

```go
import "github.com/zachbornheimer/evident-output/terminal"

drv := terminal.NewANSI(os.Stderr, terminal.WithInteractive(true), terminal.WithSize(80, 24))
out := evo.Init(evo.Config{Options: []evo.Option{evo.Terminal(drv)}})
```

### Interactive (testkit / virtual terminal)

```go
screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
clock := testkit.NewClock()
out := evo.Init(evo.Config{Options: []evo.Option{
    evo.Terminal(screen),
    evo.Clock(clock),
    evo.VisibilityDelay(150 * time.Millisecond),
    evo.MaxFrameRate(20),
}})
// Phase/Progress draw a live region; instant Done before the threshold does not flash.
// DebugHistory (default): out.Debug → durable above live (timestamp + [DEBUG]).
// DebugPane(...): rolling slog viewport in the live region; optional failure tail.
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) (DCO sign-off). Red test → green → refactor. Small conventional commits.
