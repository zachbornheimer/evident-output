# API reference

Detail behind the README quickstart: construction, config, lifecycle, the
severity dialect, evidence capture, and platform adapters. Verified against
`doc.go` and the test suite — if this drifts from behavior, the test suite is
wrong or this doc is; file it either way.

## Construction, config, lifecycle

**Construction:** `os.Exit(evo.Main(evo.Config{}, run))` with `func run(ctx context.Context, r *evo.Run) error` is the ordinary entry. `evo.Execute(ctx, cfg, run)` is the hosted counterpart and returns a `Result`. A zero `Config` is complete: `Name` defaults to the binary name; `Facades.Stdout` defaults to `os.Stdout` with ordinary TTY inference; `Output` is Human on a TTY and Plain otherwise.
**Presentation env:** where `Config` is zero, `EVO_OUTPUT=human|plain|json|jsonl` selects the encoding (`json` puts the `evo.run` document and `jsonl` the `evo.event` lines on stdout, presentation on stderr), `EVO_VERBOSE=1` selects verbose, `EVO_DEBUG=debug|info|warn|error` selects the debug journal level, and `NO_COLOR` / `FORCE_COLOR` select color. Explicit `Config` wins over env; env wins over TTY inference. Machine output is never inferred from a piped stdout.
**Machine wire formats:** `Config.Output: JSON` writes exactly one final `"evo.run"` document to stdout when the run ends; `JSONL` streams `"evo.event"` lines to stdout as they occur, `seq` strictly monotonic and the sole ordering authority, plus the same final `run.finished` line on every terminal path including failure and cancel. Either way, presentation goes to stderr, so stdout stays machine-only.
**Config honesty:** `Debug` is a stdlib `slog.Leveler` (`Debug: slog.LevelDebug`): the lowest level the debug journal keeps. Nil, the zero value, keeps the journal off unless `EVO_DEBUG` names a level; a level you name, `slog.LevelInfo` included, beats `EVO_DEBUG`.
**Lifecycle:** `evo.Main(evo.Config{}, run)` (`run` is a `RunFunc`, wired to SIGINT/SIGTERM) finishes the run and returns the derived `int` exit code, which the caller passes to `os.Exit` (`os.Exit(evo.Main(evo.Config{}, run))`; Main never exits the process itself, so a bare `evo.Main(evo.Config{}, run)` exits 0 after a failed run); `evo.Execute(ctx, cfg, run)` takes the caller's own `context.Context` and returns the full `Result` (`Conclusion` plus the application error `run` returned) instead of exiting — `Result.ExitCode()` for callers composing their own exit path or embedding Evo in a larger program. `evo.MainWith` was removed in 1.0. A non-nil `run` error is recorded as Fail only when nothing already failed. See "Lifecycles" below for the two supported shapes.
**Messages:** one human instrument — `fmt.Fprint*(r.Text(), ...)`; Facts show only with `Config.Verbose`. Infrastructure logs: `slog.New(r.SlogHandler())` (level from `Config.Debug` only), written into the debug journal, which shows on stderr — a piped run like `prog > log.txt` won't capture them; redirect with `2>` (or `2>&1`) instead. Semantic state: `Task`.
**Mutations:** `evo.Effect(ctx, evo.EffectSpec{Verb, Object, Quantity}, fn)` (an opaque mutation Evo cannot model: a git ref, a worktree, an API change), `evo.File` (file state), and `evo.Exec` pick `[planned]` vs `[changed]` from `Config.DryRun` — one spelling, never a call-site tense flip. **Planned tense:** `DryRun: true` is `--dry-run` — every mutation is planned, no mutation callback runs, and the Conclusion reads planned. Quantity records (`evo.Effect` with `EffectSpec.Quantity`) tally and always render when the run ends. A named row (`evo.File`'s `write <path>`, `evo.Exec`'s `run <executable>`) streams the instant its owning task resolves, under that task's own block, bounded by the same viewport cap and `… +N more (not shown)` overflow the run-end ledger uses. `Record`/`RecordLabel`/`RecordName` were removed in 1.1: a classification is a `Fact`, never a ledger row.

**Partial commits:** an `Effect` callback that committed part of its aggregate before failing returns `evo.PartialEffect(committed, err)`. `Effect` records one changed row with the spec's Verb and Object and `Quantity: committed` (none for 0), then returns an error that keeps `err` reachable through `errors.Is`/`errors.As`, so the Task fails while the ledger stays truthful — human rows, JSON, and the JSONL `effect.committed` payload all carry the committed count. A nil `err`, a negative `committed`, or more than `EffectSpec.Quantity` returns `ErrInvalidPartialEffect` and records nothing. Dry runs never call the callback, so they plan the full `Quantity`. `PartialEffect` is not a retry protocol and implies no rollback.
**Loops and taxonomy:** declare one named child per item under `Group`/`Sequence` (`group.Task(name)`), then `Task.Define` submits that item's atomic work — `Group.Each`/`Sequence.Each` were removed in 1.0; a Task whose `Define` returns `evo.Exclude(reason)` joins the counted, summed skip partition (the item name is the Task name): one per item Task, `group.Task(item).Define(...)`. Human output folds a Group's Excluded item children (two or more; a lone one keeps its named row) into one tally under the Group's row (`! kept N (...)` / `- skipped N (...)` — renderer copy, not a live `Kept` method; Kept was removed in 1.1) when the Group's own row names their subject — its own Task (below) or its own `Summary` — or when no sibling finished work of its own. Beside such a work peer, a Excluded child is a peer category and keeps its named row. `--verbose` lists the items under each reason; JSON/JSONL keep every child. **Own Task:** a Group's child Task named for the Group itself (`items := g.Group("branches"); work := items.Task("branches")`) is the Group's _own Task_ — the category's own work (classify, `Summary`, `Effect`), not one of its items. A Group has no `Define`, and an `Effect`'s ledger subject is its Task's name, so a category's plan (`[planned] branches  delete 87 local tips`) is owned by the Task that shares the category's name. When the own Task is the Group's only row after its items fold (no Group `Summary`, no nested Group/Sequence), the Group renders as that one row plus its tally. The own Task is never folded as an item, even when it only resolved `Excluded`; a child with any other name never stands in for its Group. `Task.Progress(completed, total).Doing(name)` sets the count and the live item name; Plain output does not stream a durable phase line per name. **One category row from per-item Effects:** give each item its own Task and let each call `evo.Effect` with the same `Verb` and `Object` (`EffectSpec{Verb: evo.Update, Object: "package", Quantity: 1}`); never sum the count by hand into one Task. The human ledger folds sibling per-item Effects that share verb and object into one row, attributed to the highest enclosing Group/Sequence whose whole subtree records that same Effect and every one of whose members contributed to it (a sibling Task that did nothing this run still counts as a member, so the row names the same container on every run; two manager Groups under "centralize packages" give `[changed] centralize packages  updated 1,663 packages`; `[planned]` under dry run), counted per item, with thousands grouping on every ledger count. A container that also holds a different verb or object, or a `File`/`Exec` row, keeps its uniform sub-Groups as their own rows, and its own direct items still fold per verb and object into a row named for the container (`npm` with five `update package` Tasks and one `delete lockfile` Task prints `updated 5 packages` and the lockfile row); a lone item renders as before. Two folded rows with the same name are qualified by container path (`alpha › branches`, `beta › branches`). A failed item adds nothing to the row except the work a `PartialEffect` reports as committed, and it still surfaces as a Failed Task row with its Problems. JSON/JSONL keep every per-item Effect. Do not pre-aggregate to dodge the row flood — that hides the per-item evidence the machine outputs carry.
**Confirm:** `r.Confirm(question, …)` (`Run.Confirm`) owns the whole ask-decide-resolve gate and returns the answer as a `bool` — a decline is `⊘ declined`, never a Go error. `question` is literal text, not a printf format (Doing is printf-variadic; Task is name-only), so build the string yourself (`fmt.Sprintf`) before calling. Return `evo.Refuse("declined")` on a decline so the run concludes Refused → exit `1` (see the README's exit-code table); check a separate flag before calling Confirm at all if declining should exit `0` instead. `evo.Destructive()` makes the default answer no. `evo.NonInteractive("--yes")` names the flag that passes the gate unattended; without a terminal and without that flag the gate is refused, and the refusal's remedy names the flag.
**Capture:** `cmd.Stdout = task.Writer()` (and stderr the same way) turns a talkative child's last line into the live doing-text and retains a bounded, redacted ring for Fail evidence. `Facades.Redactor` applies before retention. Do not clear the live region around a child.
**Platform:** `Run.ResultWriter()` keeps the domain payload on stdout and presentation on stderr.

## Lifecycles

Two entry points, both of which finish the run for you:

| Shape                         | Who exits the process                              | Use when                                                                                                                                                  |
| ----------------------------- | -------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `evo.Main(evo.Config{}, run)` | `main`, via `os.Exit(evo.Main(evo.Config{}, run))` | An ordinary CLI entrypoint — the common case. SIGINT and SIGTERM cancel the run.                                                                          |
| `evo.Execute(ctx, cfg, run)`  | The caller, after inspecting `Result.ExitCode()`   | A CLI composing its own exit path (see [exit-code-fidelity](guides/exit-code-fidelity.md)), a test, or an embedded use that owns its context and process. |

`Execute` installs no signal handlers and never exits: the caller owns the
context. Its returned `Result` carries the Conclusion (`Result.Conclusion`),
the exit code (`Result.ExitCode()`) and the application error `run` returned,
and something must do something with it (`os.Exit(result.ExitCode())`, assert
on it in a test, or fold it into a larger program's own decision). Cancelling
the context ends the run Cancelled with exit 130, including while declared
work still runs after `run` returned.

## Pick the entity

| Shape        | Use when                                                                                                                                                                                            |
| ------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Task**     | One atomic unit — its check or work submitted with `Define` (success is the callback returning `nil`); `Define` returns `Fail`, `Refuse` or `Exclude` to end it; `Problem` adds a non-fatal finding |
| **Group**    | Independent collection of atomic tasks (state is **derived**); the scheduler may overlap eligible children                                                                                          |
| **Sequence** | Ordered dependency of tasks (state is **derived**); a failed child auto-resolves later siblings to NotStarted; both Group and Sequence nest recursively via `.Sequence`/`.Group`                    |

Multi-gate: give each gate its own Task whose `Define` returns `evo.Refuse(...)` when its precondition fails, and order the mutation `After` every gate; a Refused predecessor leaves the mutation NotStarted. `Result`/`Conclusion` answer the same question once a run has finished, so no mid-run query is exported; `Main` maps `ExitCode`.

### Task is one independently meaningful promise

A Task names **one independently schedulable promise whose outcome is independently meaningful to the user** — not a display row, not a subject label, not a container reached for merely to earn a row on screen. A good Task name answers "what will this unit of work accomplish or determine?" and, as a strong heuristic (not a grammar rule review mechanically enforces), reads as an action: verb + concrete object — `check file integrity`, `format Python`, `stabilize Go source`, `lint Go`, `check Python`, `build application icons`. `file integrity` names a subject, not the work; `fix`, `go`, `pre-commit`, `classify` alone read as a category, a tool name, or a phase, not a promise. A concise contextual name can still be perfectly clear — context, not word count, decides.

Four tests settle it when the heuristic alone is ambiguous:

1. If it fails, does the Task's name alone tell the user what failed?
2. Can this unit run/wait/fail/satisfy independently?
3. Would the user care about its independent outcome?
4. Is it actual work, rather than a category, a display heading, a fact, a verification dimension, or an implementation phase?

If the answers are no, it probably is not a Task.

`Group` and `Sequence` **organize** work — they are never themselves fake work created only to earn a success row. `Task("fix")` that really owns several independently meaningful operations should become `Group("prepare staged files")` (or `Sequence`) with each operation as its own verb+object Task underneath; the container header's own visibility is a renderer decision, independent of whether the header deserves a row at all. Human output keeps every `Group` and `Sequence` row, with its children indented beneath it as `evo.run` nests them; collapse folds routine detail beneath a container, never the container's own row. Two Groups' `build` rows stay bare because each sits under its own Group row. Human output drops a finished no-op child (Done, no-work or already-satisfied, nothing else on it, no effect named for it) whenever other content is visible and the run did not fail, block or cancel. A root Task that simply finishes stays a landmark; only one proven already-satisfied by `Verify` is dropped there. A `Sequence` keeps its header and steps. JSON and JSONL always keep every Group and Task. `[planned]`/`[changed]` rows print in Task declaration order. A cancelled run ends `[cancelled]  <subject>  by user`, plus `! partial changes were applied before cancellation` only when an Effect committed.

One Task may still make several internal observations without promoting each predicate to a sibling Task: `check file integrity` can inspect merge markers, path validity, staged/worktree consistency, symlinks, and generated-file corruption, and report them all as `Fact`/`Problem` evidence under the one Task that answers a single user-meaningful question. Only split an observation into its own Task when it has an independently meaningful lifecycle/remediation and can run on its own. `Task` intentionally has no `.Task`/`.Group`/`.Sequence` child constructors — only `Run`, `Group`, and `Sequence` declare children, so a Task cannot structurally grow a container of its own; review (`API-045`) teaches the semantic half of this boundary that a compile-time signature cannot decide.

## Endings dialect

| Ending                    | Meaning                                                                                 |
| ------------------------- | --------------------------------------------------------------------------------------- |
| `Problem` (non-fatal)     | Soft concern or **optional** tool missing; the Task keeps the Outcome its return gives  |
| `return evo.Refuse(...)`  | Policy / precondition failed; **stop before mutation** (evaluation succeeded), exit `1` |
| `return evo.Fail(...)`    | Evaluation failed or **required** tool/IO failed, exit `2`                              |
| `return evo.Exclude(...)` | Policy chose not to run it; exit `0`                                                    |

A Refused Task is not a Go `error` you wrap: return `evo.Refuse(summary)` from `Define` and let `Main` exit `1`.

## One check Task, many Problems

A Task with several findings owns them all as `Problem`s — never one `Task`
per finding, never every finding flattened into a single
`errors.New(strings.Join(...))` string:

```go
func run(ctx context.Context, r *evo.Run) error {
    task := r.Task("check file integrity")
    for _, issue := range issues {
        task.Problem(issue.Summary,
            evo.On(fmt.Sprintf("%s:%d", issue.Path, issue.Line)),
            evo.Code(issue.Code),
        )
    }
    task.Define(func(context.Context) error {
        if len(issues) > 0 {
            return evo.Fail("file integrity check found problems")
        }
        return nil
    })
    return nil
}
```

`Problem(summary, opts...)` appends one non-fatal Problem and returns the
`*Task` to chain (`task.Problem(...).Problem(...)`); it never ends the Task.
A Task ends exactly once, by returning, and keeps the Outcome its return
gives with every Problem still attached: a Problem and a `nil` return are
Succeeded with exit 0, so a check that must fail on its findings returns
`evo.Fail` itself, as above.

Policy exclusion is `return evo.Exclude(reason)`; an Excluded Task is not a
failure and exits `0`. Renderer copy like `- skipped N (...)` is observed
output, not a live method.

Every accumulated Problem survives in `Result`/JSON/JSONL even when the
plain human view bounds how many render inline (5 by default) behind an
`and N more failures` line — the count is always authoritative, and a
remedy (`evo.Next(...)`) attached to any Problem
still reaches the run's own Next-steps output. See
[docs/migration/1.1.md](migration/1.1.md) for the exact 1.0→1.1 signatures.

## Child processes / tool-backed gates

Capture and Writer belong to the **Task** (whether it ran or was resolved as a
fact-check gate), not the whole session — and not `context`.
For an `*exec.Cmd`, wire stdout/stderr through `Task.Writer()`:

```go
upgrade := out.Task("brew packages")
upgrade.Define(func(ctx context.Context) error {
    cmd := exec.CommandContext(ctx, "brew", "upgrade", "--formula")
    cmd.Stdout = upgrade.Writer()
    cmd.Stderr = upgrade.Writer()
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("brew upgrade failed: %w", err)
    }
    return nil
})
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
- **Silent by default:** the ring retains; Fail with `evo.Detail` (or `return fmt.Errorf` inside Define) renders a summary/evidence split.
- **Redaction:** `Facades.Redactor` applies before ring retention.

## Platform adapters (contracts, not sugar)

Keep the core vocabulary small. Scale via **Config**, **schema keys**, and **stream contracts**:

| Need                  | Contract                                                          |
| --------------------- | ----------------------------------------------------------------- |
| Domain payload purity | `Run.ResultWriter()` (stdout payload, human on stderr)            |
| Secret scrubbing      | `Facades.Redactor` — Debug fields + capture ring                  |
| Machine consumers     | `Config.Output: JSON` or `JSONL` (stdout document or event lines) |

Avoid inventing parallel APIs (`RunAll`, framework-specific facades in core). Prefer one `Config` field over a new top-level type.

## Shared resources and concurrency

Evo coordinates shared state for you; there is no lock or unlock call.

- **`File`** claims its own path for writing while it inspects, writes, and records it.
- **`FileSpec.Inputs`** entries (on `File` and `Exec`) are observed under a read claim, so an observation sees a whole commit or none of it — never a torn write.
- **`Effect`** claims `EffectSpec.Resource` for writing while its callback runs. Name at most one: `FSResource(path)` for a file or a coarse directory (a module, a repository root), `LogicalResource(name)` for state with no truthful path (a package database, a remote).

Reads share. Any overlapping pair that includes a write waits: filesystem claims overlap when the paths are equal or one contains the other; logical claims overlap only when their names match. A claim never creates an `After` dependency and never adds to freshness. A Task waiting on a conflicting claim shows `waiting for <resource>` as its live activity; an uncontended claim renders nothing. Code holding a resource (an `Effect` callback with a `Resource`) that calls `File`, or anything else needing a second resource, fails with `ErrNestedResourceAcquisition` instead of risking deadlock.

**Guarantee scope.** Resource claims coordinate every `Output` in one process. Across processes, the manifest's exclusive lock carries the guarantee: the first `File`/`Exec` in a Run takes it before claiming any resource and holds it until the run ends, so two processes using the **same manifest namespace** (same `StateDir`, or same `AppID` and workspace) never interleave tracked `File`/`Exec` state. Different namespaces are independent by design and do not coordinate. Opaque `Effect` claims are process-local: two processes running the same `Effect` are not serialized by Evo.

## Vocabulary

| Type         | Meaning                                                                                                               |
| ------------ | --------------------------------------------------------------------------------------------------------------------- |
| `Task`       | One atomic unit — submitted with Define; it ends by returning (`Fail`, `Refuse`, `Exclude` or `nil`)                  |
| `Group`      | Independent collection of tasks (state is **derived**); scheduler may overlap eligible children                       |
| `Sequence`   | Ordered dependency of tasks (state is **derived**); failure cascades to NotStarted                                    |
| `Problem`    | Structured evidence for warn / block / fail; a Task accumulates many via `Problem(...)` before it resolves once       |
| `Effect`     | One opaque mutation (`EffectSpec{Verb, Object, Quantity}`); `Config.DryRun` picks planned vs changed at one call site |
| `Conclusion` | Headline + `Changed` / `Partial` / `Cancelled` + exit code                                                            |
| `Main`       | process exit code for CLI entrypoints                                                                                 |

Evo owns scheduling through Group, Sequence, Define, and After (`Group.Each`/`Sequence.Each` were removed in 1.0).

`After(g)` on a Group or Sequence waits for the Tasks declared into it. A collection already populated when it is named in `After` (or as the step before in a Sequence) is taken as declared: that edge waits for the Tasks declared so far, and a Task declared into `g` later never gates it. One named while still empty stays open, so a Task wired `After(g)` before the loop that fills `g` waits for every child; `g.Wait()`, a Wait on the dependent, or the end of the run closes it, and an empty collection closed that way counts as done. A Task declared into a Sequence step the Sequence has already moved past runs after the Sequence's latest step and becomes its latest step, so a Sequence still runs one step at a time in declaration order.

## Status

**Architecture spec:** [v0.5](architecture/EVIDENT_OUTPUT_ARCHITECTURE_SPEC_v0.5.md) (design candidate).
**Implemented surface:** ordinary ladder through Effect/File/Capture/slog/ResultWriter; interactive VT; hardened MCP; polish-phase docs under `docs/`. External/manual items remain waived (Windows ConPTY / tmux / SSH RC, a11y contrast / screen-reader, host RC matrices and a11y manual reviews).

| Ready now                                                                                                                   | External / manual only                |
| --------------------------------------------------------------------------------------------------------------------------- | ------------------------------------- |
| Task, Group, Sequence, Effect, File, Print                                                                                  | Windows ConPTY RC (PORT-003)          |
| Conclusion + exit codes + Cancel cleanup                                                                                    | tmux RC (PORT-004)                    |
| Plain, JSON (§25.1), JSONL (§25.2)                                                                                          | SSH RC (PORT-005)                     |
| Interactive live region (`testkit.Screen`)                                                                                  | Light/dark contrast review (A11Y-006) |
| `SlogHandler`, `DebugWriter`, `Suspend`, `Snapshots()`                                                                      | Screen-reader review (A11Y-007)       |
| Appendix H.1–H.22 + agent harness + multi-file GoPackage review                                                             | —                                     |
| ANSI driver + width/CJK + OSC strip + s390x cross-compile                                                                   | —                                     |
| CLI: `review` / `preview` / `explain` (real JSON)                                                                           | —                                     |
| MCP: lifecycle, protocol negotiate, unknown-field reject, panic contain, token budget, remote-path reject, catalog checksum | —                                     |
| Framework adapter examples (urfave/Kong shapes, no core deps)                                                               | —                                     |

Completeness vs §31 (v0.3 matrix): [`architecture/COMPLETENESS_MATRIX.md`](architecture/COMPLETENESS_MATRIX.md).
Conformance detail (traceability, scenarios): [`../conformance/TRACEABILITY.md`](../conformance/TRACEABILITY.md).
