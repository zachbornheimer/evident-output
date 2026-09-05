# API reference

Detail behind the README quickstart: construction, config, lifecycle, the
severity dialect, evidence capture, and platform adapters. Verified against
`doc.go` and the test suite — if this drifts from behavior, the test suite is
wrong or this doc is; file it either way.

## Construction, config, lifecycle

**Construction:** `evo.Init(Config{…})` is the sole constructor — the package-level default instance (front door) by default; `Config.Isolated: true` returns an independent hosted instance instead — TTY, `NO_COLOR`, stdout/stderr defaults included. Advanced: `Config.Options: []Option{Title(...), …}` for exact writer/terminal/clock wiring — an explicit `To(w)` under `Options` bypasses TTY/color inference entirely (raw ANSI on `w` unless you also call `NoColor()`); leaving both `To()` and `Terminal(...)` unset instead defaults to `os.Stdout` with the ordinary TTY/`NO_COLOR` inference applied.
**Config honesty:** `VisibilityDelay: evo.Delay(0)` is immediate (nil = default 80ms). `Debug: evo.DebugConfig{Level: evo.LevelDebug}` selects the journal threshold — `evo.LogLevel`, a distinct type from stdlib `slog.Level` (`LevelUnset` → Info).
**Lifecycle:** `evo.Main(run)` (default instance, `run func() error`) or `evo.MainWith(out, run)` (hosted, `Config.Isolated: true`, `run func(*Output) error`) exits the process itself after Finish + Close; `evo.Run(run)` / `out.Run(run)` return the exit code instead of exiting, for callers composing their own exit path. A non-nil `run` error is recorded as Fail only when nothing already failed. See "Lifecycles" below for the three supported shapes, including `Init` with no `Main`/`Run` at all.
**Messages:** one human instrument — `Print` / `Printf` / `Println` + `Verbose()`. Infrastructure logs: `slog.New(out.SlogHandler())` (level from `Config.Debug.Level` only), written to `Config.Stderr` (default `os.Stderr`) — a piped run like `prog > log.txt` won't capture them; redirect with `2>` (or `2>&1`) instead. Semantic state: `Task`.
**Mutations:** `Task.Add/Delete/Create/Update/Remove/Write/Push/Record/RecordName` pick `[planned]` vs `[changed]` from `Config.DryRun` — one spelling, never a call-site tense flip. Quantity records (`Record`, the mutation verbs with `evo.Affected(n)`) tally and always render at `Finish`. `RecordName` names one item individually — it streams its row the instant its owning task resolves (`Done`/`Fail`/`Block`), under that task's own block, bounded by the same viewport cap and `… +N more (not shown)` overflow the Finish ledger uses.
**Loops and taxonomy:** `Task.Each(items []string)` / `EachN(len(items))` (any other slice type) own absolute progress; `Task.Skipped(reason, name)` / `Task.Kept(reason, name)` own the counted, summed skip/keep partition.
**Confirm:** `evo.Confirm(question, …)` owns the whole ask-decide-resolve gate — `Done` / `⊘ declined` / `⊘ blocked by policy`, never a Go error. `question` is literal text, not a printf format — Confirm is the one entity-text spelling that takes no variadic fmt args (every other one — Task/Done/Warn/Doing/Skip/Sequence/DisplayGroup/Reason — is printf-variadic), so build the string yourself (`fmt.Sprintf`) before calling. A decline resolves `[blocked]` → exit `1` (see the README's exit-code table) — pass `AssumeYes` (or check a separate flag before calling Confirm at all) if declining should exit `0` instead. The default policy hint names a `--yes` flag; pass `evo.PolicyFlag("--apply")` when your program's real flag is spelled differently.
**Capture:** `Task.Evidence` (work or tool-backed gate); silent by default; pending fragments in `DetailTail`; `Config.Redactor` before retention. `cmd.Stdout = task.Writer()` turns a talkative child's last line into the live doing-text; `out.Suspend(fn)` hands the tty to a child that paints its own UI.
**Platform:** `evo.ID` + narrow `Scope` (Task only — not a sandbox); `ResultWriter()` under `FormatData`.

## Lifecycles

Three supported shapes, all built on the same `Finish` (validate + compute
Conclusion) → `Close` (idempotent cleanup) sequence:

| Shape                      | Who calls Finish/Close                                                                                                         | Who exits the process                                             | Use when                                                                                                                                                                       |
| -------------------------- | ------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `Init` + `Main`/`MainWith` | Main/MainWith, automatically                                                                                                   | Main/MainWith, via `os.Exit`                                      | An ordinary CLI entrypoint — the common case                                                                                                                                   |
| `Init` + `Run`/`out.Run`   | Run/out.Run, automatically                                                                                                     | The caller, after inspecting `code`                               | A CLI composing its own exit path (see [exit-code-fidelity](guides/exit-code-fidelity.md)), or a test that needs the code without exiting the test binary                      |
| `Init` alone (no Main/Run) | The caller, explicitly (`out.Finish()` then `out.Close()`, or just `defer out.Close()` — Close runs Finish itself when needed) | Never — evo never exits a process it didn't arm via Main/MainWith | A hosted/embedded use (a library, a long-running service, an MCP tool handler) that owns its own process lifetime and only wants evo's presentation, not its exit-code opinion |

`Run` (`evo.Run` / `out.Run`) is the non-exiting reconciler every other
lifecycle is built from — it is never itself a "fire and forget" call:
its returned `int` is the Conclusion's exit code, and something must do
something with it (return it from `main` via `os.Exit`, assert on it in a
test, or fold it into a larger program's own decision).

For `Init` alone: nothing renders the final Conclusion band, and evidence
capture / redaction never flush, until `Finish` runs — an embedding caller
that forgets to call it (or `Close`, which calls it for you) gets an Output
that never reports its own outcome. `Close` is safe to call unconditionally
and more than once (idempotent); prefer `defer out.Close()` right after
`Init` so every return path — including a panic — still finalizes.

## Pick the entity

| Shape            | Use when                                                                                                                                                                                                    |
| ---------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Task**         | Everything — a check/gate resolved directly (`Done`/`Warn`/`Block`/`Fail`/`Skip`, no `Doing`/`Progress`) renders as a fact row; work with phases, progress, or mutation verbs shows a spinner while running |
| **DisplayGroup** | Presentation-only collection of independent tasks (state is **derived**); concurrent Running children expected                                                                                              |
| **Sequence**     | Ordered dependency of tasks (state is **derived**); a failed child auto-resolves later siblings to NotStarted; both DisplayGroup and Sequence nest recursively via `.Sequence`/`.DisplayGroup`              |

Multi-gate: resolve every Task, tracking a local `blocked` bool at each `Block` call site, then `if blocked { return nil }` before mutation — `Output.Run`/`Conclusion` answer the same question once a run has finished, so no mid-run query is exported; `Main` maps `ExitCode`.

## Severity dialect

| Outcome   | Meaning                                                                       |
| --------- | ----------------------------------------------------------------------------- |
| **Warn**  | Soft concern or **optional** tool missing; command may continue               |
| **Block** | Policy / precondition failed; **stop before mutation** (evaluation succeeded) |
| **Fail**  | Evaluation failed or **required** tool/IO failed                              |

`Block` ≠ Go `error`. After Block, return nil from `run` and let `Main` exit `1`.

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

Tool-backed **condition** (a `Task` resolved directly, no `Doing`/`Progress`):

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

## Vocabulary

| Type           | Meaning                                                                                                              |
| -------------- | -------------------------------------------------------------------------------------------------------------------- |
| `Task`         | One operation — a named condition resolved directly (Done/Warn/Block/Fail/Skip) or work with phase/progress          |
| `DisplayGroup` | Independent collection of tasks (state is **derived**)                                                               |
| `Sequence`     | Ordered dependency of tasks (state is **derived**); failure cascades to NotStarted                                   |
| `Problem`      | Structured evidence for warn / block / fail                                                                          |
| Mutation verbs | `Add`/`Delete`/`Create`/`Update`/`Remove`/`Write`/`Push` — effects that happened vs would happen, from one call site |
| `Conclusion`   | Headline + `Changed` / `Partial` / `Cancelled` + exit code                                                           |
| `Main`         | Finish + Close + process exit code for CLI entrypoints                                                               |

Do **not** put schedulers, `RunAll`, retries, or shell execution in this library. Review rule **API-026** flags those helpers only on evo receivers (AST), not `strings.Map`.

## Status

**Architecture spec:** [v0.5](architecture/EVIDENT_OUTPUT_ARCHITECTURE_SPEC_v0.5.md) (design candidate).
**Implemented surface:** ordinary ladder through mutation verbs/Capture/slog/ResultWriter; interactive VT; hardened MCP; polish-phase docs under `docs/`. External/manual items remain waived (Windows ConPTY / tmux / SSH RC, a11y contrast / screen-reader, host RC matrices and a11y manual reviews).

| Ready now                                                                                                                   | External / manual only                |
| --------------------------------------------------------------------------------------------------------------------------- | ------------------------------------- |
| Task, DisplayGroup, Sequence, mutation verbs, Print                                                                         | Windows ConPTY RC (PORT-003)          |
| Conclusion + exit codes + Cancel cleanup                                                                                    | tmux RC (PORT-004)                    |
| Plain, JSON (§25.1), JSONL (§25.2)                                                                                          | SSH RC (PORT-005)                     |
| Interactive live region (`testkit.Screen`)                                                                                  | Light/dark contrast review (A11Y-006) |
| `SlogHandler`, `DebugWriter`, `Suspend`, `Snapshots()`, `MaxEntities`, `MaxEvents`, `AlsoWrite`                             | Screen-reader review (A11Y-007)       |
| Appendix H.1–H.22 + agent harness + multi-file GoPackage review                                                             | —                                     |
| ANSI driver + width/CJK + OSC strip + s390x cross-compile                                                                   | —                                     |
| CLI: `review` / `preview` / `explain` (real JSON)                                                                           | —                                     |
| MCP: lifecycle, protocol negotiate, unknown-field reject, panic contain, token budget, remote-path reject, catalog checksum | —                                     |
| Framework adapter examples (urfave/Kong shapes, no core deps)                                                               | —                                     |

Completeness vs §31 (v0.3 matrix): [`architecture/COMPLETENESS_MATRIX.md`](architecture/COMPLETENESS_MATRIX.md).
Conformance detail (traceability, scenarios): [`../conformance/TRACEABILITY.md`](../conformance/TRACEABILITY.md).
