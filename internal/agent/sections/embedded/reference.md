# API reference

Detail behind the README quickstart: construction, config, lifecycle, the
severity dialect, evidence capture, and platform adapters. Verified against
`doc.go` and the test suite — if this drifts from behavior, the test suite is
wrong or this doc is; file it either way.

## Construction, config, lifecycle

**Construction:** `evo.Init(Config{…})` is the sole _Output_ constructor — the package-level default instance (front door) by default; `Config.Isolated: true` returns an independent hosted instance instead — TTY, `NO_COLOR`, stdout/stderr defaults included. Advanced: Config fields (`Stdout`, `Plain`, `Color`, `DryRun`, `Preview`, `Title`) are the ordinary wiring; leaving `Stdout` unset defaults to `os.Stdout` with the ordinary TTY/`NO_COLOR` inference applied.
**Presentation env:** when the matching Config field is still zero, `EVO_OUTPUT=human|plain|json|jsonl|stream-json` (`stream_json` accepted) selects encoding, `EVO_COLOR=auto|always|never` selects color, `EVO_VERBOSE=1` selects verbose, and `EVO_DEBUG=info|debug|trace` selects the debug journal level. Explicit Config/Options win over env; env wins over TTY inference. `Format` still only routes streams — `FormatData` keeps domain payload on stdout and puts json/jsonl/stream-json on stderr. Empty `EVO_COLOR` plus `NO_COLOR` still means never-color. `Config.Plain: true` is not cleared by `EVO_OUTPUT=human`.
**Machine wire formats (spec §32.1/§35/§38):** `Config.Format: FormatJSON` writes exactly one final `"evo.run"` document (§35) to `Config.Stdout` at `Finish`; `FormatJSONL` streams `"evo.event"` JSONL lines to `Config.Stdout` as they occur (§38 — every required family, `seq` strictly monotonic and the sole ordering authority), plus the same final `run.finished` line on every terminal path including failure and cancel. Either way, human presentation still goes to `Config.Stderr`, exactly like `FormatData` — stdout stays machine-only. `evo.ParseFormat(s)` parses a host CLI's own `--format`/`--json` flag value (`"human"|"data"|"external"|"json"|"jsonl"`) into a `Format`; Evo never infers `FormatJSON` merely because stdout is a pipe — the caller states it explicitly, always. `evo.WriteJSON(w, result)` is `FormatJSON`'s document without owning stdout, for HTTP/embedding call sites that already have a `Result` in hand.
**Config honesty:** `VisibilityDelay: evo.Delay(0)` is immediate (nil = default 80ms). `Debug: evo.DebugConfig{Level: evo.LevelDebug}` selects the journal threshold — `evo.LogLevel`, a distinct type from stdlib `slog.Level` (`LevelUnset` → Info).
**Lifecycle:** `evo.Main(run)` (default instance, `run func(context.Context) error`, wired to SIGINT/SIGTERM) exits the process itself after Finish + Close and returns only the derived `int` exit code; `evo.Run(ctx, run)` / `out.Run(ctx, run)` take the caller's own `context.Context` and return the full `Result` (`Conclusion` plus the application error `run` returned) instead of exiting — `Result.ExitCode()` for callers composing their own exit path (`Config.Isolated: true` for a hosted `*Output`). `evo.MainWith` was removed in 1.0; an `Isolated *Output` now calls its own `Output.Run` instead. A non-nil `run` error is recorded as Fail only when nothing already failed. See "Lifecycles" below for the three supported shapes, including `Init` with no `Main`/`Run` at all.
**Messages:** one human instrument — `Print` / `Printf` / `Println` + `Verbose()`. Infrastructure logs: `slog.New(out.SlogHandler())` (level from `Config.Debug.Level` only), written to `Config.Stderr` (default `os.Stderr`) — a piped run like `prog > log.txt` won't capture them; redirect with `2>` (or `2>&1`) instead. Semantic state: `Task`.
**Mutations:** `evo.Effect(ctx, evo.EffectSpec{Verb, Object, Quantity}, fn)` (an opaque mutation Evo cannot model: a git ref, a worktree, an API change), `evo.File` (file state), and `evo.Exec` pick `[planned]` vs `[changed]` from `Config.DryRun` or `Config.Preview` — one spelling, never a call-site tense flip. **Planned tense, two announcements:** `DryRun: true` is `--dry-run` — it opens `[dry-run] <Subject>` and really does stop. `Preview: true` is the plan a confirm gate is about to act on — same skipped Effect callbacks and same `[planned]` ledger, but the header is your `Subject` alone (`repo <path>`) and no `[dry-run]` tag, because telling the user nothing will happen and then asking them to authorize it is a contradiction. Both suppress the trailing band on a pure planned verdict when a `Subject` header rendered. Quantity records (`evo.Effect` with `EffectSpec.Quantity`) tally and always render at `Finish`. A named row (`evo.File`'s `write <path>`, `evo.Exec`'s `run <executable>`) streams the instant its owning task resolves, under that task's own block, bounded by the same viewport cap and `… +N more (not shown)` overflow the Finish ledger uses. `Record`/`RecordLabel`/`RecordName` were removed in 1.1: a classification is a `Fact`, never a ledger row.

**Partial commits:** an `Effect` callback that committed part of its aggregate before failing returns `evo.PartialEffect(committed, err)`. `Effect` records one changed row with the spec's Verb and Object and `Quantity: committed` (none for 0), then returns an error that keeps `err` reachable through `errors.Is`/`errors.As`, so the Task fails while the ledger stays truthful — human rows, JSON, and the JSONL `effect.committed` payload all carry the committed count. A nil `err`, a negative `committed`, or more than `EffectSpec.Quantity` returns `ErrInvalidPartialEffect` and records nothing. Dry runs never call the callback, so they plan the full `Quantity`. `PartialEffect` is not a retry protocol and implies no rollback.
**Loops and taxonomy:** declare one named child per item under `Group`/`Sequence` (`group.Task(name)`), then `Task.Define` submits that item's atomic work — `Group.Each`/`Sequence.Each` were removed in 1.0; `Task.Skipped(reason)` / `Task.Kept(reason)` own the counted, summed skip/keep partition (the item name is the Task name). `Task.Step(completed, total, name)` sets the count and the live item name together under one lock; Isolated+Plain does not stream a durable phase line per name.
**Confirm:** `evo.Confirm(question, …)` owns the whole ask-decide-resolve gate — `Done` / `⊘ declined` / `⊘ blocked by policy`, never a Go error. `question` is literal text, not a printf format — Confirm is the one entity-text spelling that takes no variadic fmt args (every other one — Task/Warn/Doing/Sequence/Group/Reason — is printf-variadic), so build the string yourself (`fmt.Sprintf`) before calling. A decline resolves `[blocked]` → exit `1` (see the README's exit-code table) — pass `AssumeYes` (or check a separate flag before calling Confirm at all) if declining should exit `0` instead. The default policy hint names a `--yes` flag; pass `evo.PolicyFlag("--apply")` when your program's real flag is spelled differently.
**Capture:** `cmd.Stdout = task.Writer()` (and stderr the same way) turns a talkative child's last line into the live doing-text and retains a bounded, redacted ring for Fail evidence. `Config.Redactor` applies before retention. Do not clear the live region around a child.
**Platform:** `Format: FormatData` keeps domain payload on stdout and presentation on stderr.

## Lifecycles

Three supported shapes, all built on the same `Finish` (validate + compute
Conclusion) → `Close` (idempotent cleanup) sequence:

| Shape                      | Who calls Finish/Close                                                                                                         | Who exits the process                                    | Use when                                                                                                                                                                       |
| -------------------------- | ------------------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `Init` + `Main`            | Main, automatically                                                                                                            | Main, via `os.Exit`                                      | An ordinary CLI entrypoint — the common case                                                                                                                                   |
| `Init` + `Run`/`out.Run`   | Run/out.Run, automatically                                                                                                     | The caller, after inspecting `Result.ExitCode()`         | A CLI composing its own exit path (see [exit-code-fidelity](guides/exit-code-fidelity.md)), or a test that needs the code without exiting the test binary                      |
| `Init` alone (no Main/Run) | The caller, explicitly (`out.Finish()` then `out.Close()`, or just `defer out.Close()` — Close runs Finish itself when needed) | Never — evo never exits a process it didn't arm via Main | A hosted/embedded use (a library, a long-running service, an MCP tool handler) that owns its own process lifetime and only wants evo's presentation, not its exit-code opinion |

`Run` (`evo.Run` / `out.Run`) is the non-exiting reconciler every other
lifecycle is built from — it is never itself a "fire and forget" call:
its returned `Result` carries the Conclusion's exit code (`Result.ExitCode()`)
plus the application error `run` returned, and something must do something
with it (`os.Exit(result.ExitCode())`, assert on it in a test, or fold it
into a larger program's own decision).

For `Init` alone: nothing renders the final Conclusion band, and evidence
capture / redaction never flush, until `Finish` runs — an embedding caller
that forgets to call it (or `Close`, which calls it for you) gets an Output
that never reports its own outcome. `Close` is safe to call unconditionally
and more than once (idempotent); prefer `defer out.Close()` right after
`Init` so every return path — including a panic — still finalizes.

## Pick the entity

| Shape        | Use when                                                                                                                                                                         |
| ------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Task**     | One atomic unit — its check or work submitted with `Define` (success is the callback returning `nil`); `Warn`/`Block`/`Fail`/`Skipped` state a condition directly                |
| **Group**    | Independent collection of atomic tasks (state is **derived**); the scheduler may overlap eligible children                                                                       |
| **Sequence** | Ordered dependency of tasks (state is **derived**); a failed child auto-resolves later siblings to NotStarted; both Group and Sequence nest recursively via `.Sequence`/`.Group` |

Multi-gate: resolve every Task, tracking a local `blocked` bool at each `Block` call site, then `if blocked { return nil }` before mutation — `Output.Run`/`Conclusion` answer the same question once a run has finished, so no mid-run query is exported; `Main` maps `ExitCode`.

### Task is one independently meaningful promise

A Task names **one independently schedulable promise whose outcome is independently meaningful to the user** — not a display row, not a subject label, not a container reached for merely to earn a row on screen. A good Task name answers "what will this unit of work accomplish or determine?" and, as a strong heuristic (not a grammar rule review mechanically enforces), reads as an action: verb + concrete object — `check file integrity`, `format Python`, `stabilize Go source`, `lint Go`, `check Python`, `build application icons`. `file integrity` names a subject, not the work; `fix`, `go`, `pre-commit`, `classify` alone read as a category, a tool name, or a phase, not a promise. A concise contextual name can still be perfectly clear — context, not word count, decides.

Four tests settle it when the heuristic alone is ambiguous:

1. If it fails, does the Task's name alone tell the user what failed?
2. Can this unit run/wait/fail/satisfy independently?
3. Would the user care about its independent outcome?
4. Is it actual work, rather than a category, a display heading, a fact, a verification dimension, or an implementation phase?

If the answers are no, it probably is not a Task.

`Group` and `Sequence` **organize** work — they are never themselves fake work created only to earn a success row. `Task("fix")` that really owns several independently meaningful operations should become `Group("prepare staged files")` (or `Sequence`) with each operation as its own verb+object Task underneath; the container header's own visibility is a renderer decision, independent of whether the header deserves a row at all. Human output drops a `Group` header that has no `Summary` of its own (its children render as siblings; while children are still running the live header stays, because its `N/M complete` count is the Group's own progress), and drops a finished no-op child (Done, no-work or already-satisfied, nothing else on it, no effect named for it) whenever other content is visible and the run did not fail, block or cancel. A root Task that simply finishes stays a landmark; only one proven already-satisfied by `Verify` is dropped there. A `Sequence` keeps its header and steps. JSON and JSONL always keep every Group and Task. `[planned]`/`[changed]` rows print in Task declaration order. A cancelled run ends `[cancelled] <subject>  by user`, plus `! partial changes were applied before cancellation` only when an Effect committed.

One Task may still make several internal observations without promoting each predicate to a sibling Task: `check file integrity` can inspect merge markers, path validity, staged/worktree consistency, symlinks, and generated-file corruption, and report them all as `Fact`/`Warn`/`Problem` evidence under the one Task that answers a single user-meaningful question. Only split an observation into its own Task when it has an independently meaningful lifecycle/remediation and can run on its own. `TaskHandle` intentionally has no `.Task`/`.Group`/`.Sequence` child constructors — only `Output`, `GroupHandle`, and `SequenceHandle` declare children, so a Task cannot structurally grow a container of its own; review (`API-045`) teaches the semantic half of this boundary that a compile-time signature cannot decide.

## Severity dialect

| Outcome   | Meaning                                                                       |
| --------- | ----------------------------------------------------------------------------- |
| **Warn**  | Soft concern or **optional** tool missing; command may continue               |
| **Block** | Policy / precondition failed; **stop before mutation** (evaluation succeeded) |
| **Fail**  | Evaluation failed or **required** tool/IO failed                              |

`Block` ≠ Go `error`. After Block, return nil from `run` and let `Main` exit `1`.

## One check Task, many Problems

A Task with several findings owns them all as `Problem`s — never one `Task`
per finding, never every finding flattened into a single
`errors.New(strings.Join(...))` string:

```go
task := out.Task("file integrity")
for _, issue := range issues {
    task.Problem(issue.Summary,
        evo.On(issue.Path),
        evo.Code(issue.Code),
        evo.Location(issue.Path, issue.Line, 0),
    )
}
task.Define(func(context.Context) error { return nil })
```

`Problem(summary, opts...)` appends one blocking Problem and returns
`*TaskHandle` to chain (`task.Problem(...).Problem(...)`); it does not
resolve the task. If `Define`'s callback returns `nil` while the Task has
accumulated Problems, the Task resolves
**Failed**, not Done: accumulated blocking evidence always overrides a
claimed clean outcome. The Task still resolves exactly once regardless of
how many Problems it owns.

`Warn(summary, opts...)` takes the same `ProblemOption`s (`Detail`, `Code`,
`On`, `Location`, `Next`, ...) for a non-blocking finding with the same
structured metadata — it never resolves the task either.

Every accumulated Problem survives in `Snapshot`/JSON/JSONL even when the
plain human view bounds how many render inline (5 by default) behind an
`and N more failures` line — the count is always authoritative, and a
remedy (`evo.Next(...)`/`evo.NextCommand(...)`) attached to any Problem
still reaches the run's own Next-steps output. See
[docs/migration/1.1.md](migration/1.1.md) for the exact 1.0→1.1 signatures.

## Child processes / tool-backed gates

Evidence belongs to the **entity** (a `Task`, whether it ran or was resolved as a
fact-check gate), not the whole session — and not `context`.
For an `*exec.Cmd`, wire stdout/stderr through `Task.Writer()`:

```go
upgrade := out.Task("brew packages")
cmd := exec.Command("brew", "upgrade", "--formula")
cmd.Stdout = upgrade.Writer()
cmd.Stderr = upgrade.Writer()
if err := cmd.Run(); err != nil {
    return upgrade.Failf("brew upgrade failed: %w", err)
}
```

Tool-backed **condition** (a `Task` whose check is its `Define` callback, no `Doing`/`Progress`):

```go
docker := out.Task("docker daemon")
docker.Define(func(ctx context.Context) error {
    if err := pingDocker(); err != nil {
        return fmt.Errorf("could not inspect the daemon: %w", err)
    }
    return nil
})
```

- **Ownership:** `Task.Writer()` associates child output with that entity.
- **Silent by default:** the ring retains; Failf's trailing `%w` renders a summary/evidence split.
- **Redaction:** `Config.Redactor` applies before ring retention.

## Platform adapters (contracts, not sugar)

Keep the core vocabulary small. Scale via **Config**, **schema keys**, and **stream contracts**:

| Need                  | Contract                                               |
| --------------------- | ------------------------------------------------------ |
| Domain payload purity | `Format: FormatData` (stdout payload, human on stderr) |
| Secret scrubbing      | `Config.Redactor` — Debug fields + capture ring        |
| Host-owned rendering  | `FormatExternal` + `out.Snapshot()` (no inline stream) |

Avoid inventing parallel APIs (`RunAll`, framework-specific facades in core). Prefer one `Config` field or `EntityOption` over a new top-level type.

## Shared resources and concurrency

Evo coordinates shared state for you; there is no lock or unlock call.

- **`File`** claims its own path for writing while it inspects, writes, and records it.
- **`FSPath` Basis** entries (on `File` and `Exec`) are observed under a read claim, so an observation sees a whole commit or none of it — never a torn write.
- **`Effect`** claims `EffectSpec.Resource` for writing while its callback runs. Name at most one: `FSResource(path)` for a file or a coarse directory (a module, a repository root), `LogicalResource(name)` for state with no truthful path (a package database, a remote).

Reads share. Any overlapping pair that includes a write waits: filesystem claims overlap when the paths are equal or one contains the other; logical claims overlap only when their names match. A claim never creates an `After` dependency and never adds to freshness. A Task waiting on a conflicting claim shows `waiting for <resource>` as its live activity; an uncontended claim renders nothing. Code holding a resource (an `Effect` callback with a `Resource`) that calls `File`, or anything else needing a second resource, fails with `ErrNestedResourceAcquisition` instead of risking deadlock.

**Guarantee scope.** Resource claims coordinate every `Output` in one process. Across processes, the manifest's exclusive lock carries the guarantee: the first `File`/`Exec` in a Run takes it before claiming any resource and holds it until `Close`, so two processes using the **same manifest namespace** (same `StateDir`, or same `AppID` and workspace) never interleave tracked `File`/`Exec` state. Different namespaces are independent by design and do not coordinate. Opaque `Effect` claims are process-local: two processes running the same `Effect` are not serialized by Evo.

## Vocabulary

| Type           | Meaning                                                                                                              |
| -------------- | -------------------------------------------------------------------------------------------------------------------- |
| `Task`         | One atomic unit — submitted with Define, or stated directly (Warn/Block/Fail/Skipped)                                |
| `Group`        | Independent collection of tasks (state is **derived**); scheduler may overlap eligible children                      |
| `Sequence`     | Ordered dependency of tasks (state is **derived**); failure cascades to NotStarted                                   |
| `Problem`      | Structured evidence for warn / block / fail; a Task accumulates many via `Problem(...)` before it resolves once      |
| Mutation verbs | `Add`/`Delete`/`Create`/`Update`/`Remove`/`Write`/`Push` — effects that happened vs would happen, from one call site |
| `Conclusion`   | Headline + `Changed` / `Partial` / `Cancelled` + exit code                                                           |
| `Main`         | Finish + Close + process exit code for CLI entrypoints                                                               |

Evo owns scheduling through Group, Sequence, Define, and After (`Group.Each`/`Sequence.Each` were removed in 1.0). Review rule **API-026** flags caller-invented `RunAll`/`Map`/`Retry` only on evo receivers (AST), not `strings.Map`, and does not flag Group/Sequence/Define/After.

## Status

**Architecture spec:** [v0.5](architecture/EVIDENT_OUTPUT_ARCHITECTURE_SPEC_v0.5.md) (design candidate).
**Implemented surface:** ordinary ladder through Effect/File/Capture/slog/ResultWriter; interactive VT; hardened MCP; polish-phase docs under `docs/`. External/manual items remain waived (Windows ConPTY / tmux / SSH RC, a11y contrast / screen-reader, host RC matrices and a11y manual reviews).

| Ready now                                                                                                                   | External / manual only                |
| --------------------------------------------------------------------------------------------------------------------------- | ------------------------------------- |
| Task, Group, Sequence, Effect, File, Print                                                                                  | Windows ConPTY RC (PORT-003)          |
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
