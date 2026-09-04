# Domain vocabulary

Binding nouns and verbs for Evident Output call sites.
Source: `docs/roadmap/implementation-basis.md` §5, §8 (RULE-001…007), §4 (PHIL-005).

Cross-links: [jazz-syntax.md](./jazz-syntax.md) · [presentation-boundary.md](./presentation-boundary.md)

---

## Task / Tasks

One entity, one constructor. A `Task` answers both questions "is this state
acceptable?" and "how is this work going?" — which one depends on how it's
used, not on a separate type:

| Noun      | Meaning                                                                                                                                              |
| --------- | ---------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Task**  | A named condition or unit of work — resolved directly (Done/Warn/Block/Fail/Skip) for a **condition**, or driven through Phase/Progress for **work** |
| **Tasks** | A **collection** of Tasks; collection state is **derived** from children                                                                             |

```go
gate := out.Task("working tree", evo.ID("repo.working-tree")) // condition: resolved directly below
work := out.Task("download", evo.ID("install.download"))      // work: driven through Phase/Progress
packages := out.DisplayGroup("packages")
```

(Shipped v0.2.x code spelled the condition shape `Item` — folded into `Task`
one entity, one constructor; `ItemHandle` is now a deprecated zero-cost
alias of `TaskHandle`.)

---

## Warn / Block / Fail

Severity on conditions and terminal outcomes on work — the same four verbs
either way:

| Verb      | User meaning                                          |
| --------- | ----------------------------------------------------- |
| **Done**  | Condition holds; work succeeded                       |
| **Warn**  | Proceed, but notice this                              |
| **Block** | Stop until the user acts (not necessarily a Go error) |
| **Fail**  | Operation failed                                      |

```go
gate.Done()
gate.Warn("contains ignored files", evo.Detail("2 files"))
gate.Block("contains local changes", evo.Detail("stash or commit them"))
return gate.Failf("could not inspect working tree: %w", err)
```

Structured evidence for one resolution uses `ProblemOption`s on the same call
(`evo.On(subject)`, `evo.Count(n)`, `evo.Detail(text)`, …) — a Task resolves
once, with one Problem:

```go
gate.Block("contains local changes", evo.On("working tree"), evo.Detail("stash or commit them"))
```

---

## Problem / Detail / Failf evidence

| Piece        | Audience            | Role                                                                  |
| ------------ | ------------------- | --------------------------------------------------------------------- |
| **Problem**  | Structured evidence | Subject + summary (+ optional pieces) for one failure unit            |
| **Detail**   | **User-facing**     | What the human should know or do                                      |
| **Failf %w** | **User-facing**     | Wrapped error's text, rendered as one evidence line under the summary |

PHIL-005: a trailing `": %w"`/`", %w"` on `Failf`/`Blockf` splits the formatted text into the
rendered summary and an evidence line for the wrapped error — both user-facing. Use `Detail`
for stable guidance text that isn't derived from an error. Do not bury the only user message in
a wrapped error alone with an empty summary.

```go
// Right
item.Block("contains local changes", evo.Detail("stash or commit them"))
return task.Failf("download failed: %w", err)

// Wrong — user message only in the wrapped error, empty human summary
return task.Failf(": %w", err)
```

`evo.Cause` (a `ProblemOption` from before this split existed) is deprecated: it no longer
affects the returned error since `Fail`/`Block` are statement-form.

---

## Plan vs Changes

| Construct   | Tense             | When                                       |
| ----------- | ----------------- | ------------------------------------------ |
| **Plan**    | Future / intended | Dry-run, proposed effects, not yet durable |
| **Changes** | Past / durable    | Effects that happened (or were committed)  |

```go
out.Plan("dependencies").
    Add(14, "packages").
    Remove(3, "packages")

out.Changes("dependencies").
    Added(14, "packages").
    Removed(3, "packages")
```

RULE-005: dry-run uses Plan, never simulated Tasks. Completion uses Changes for durable effects.

---

## RULE-001 — Domain verbs over generic verbs

Use built-in Changes/Plan verbs only when they are the **real** domain verb.

```go
// Wrong — "Added" is not what happened
changes.Added(1, "files placed")

// Right — Record the domain verb; object is the final grammatical object
changes.Record("placed", n, noun(n, "file", "files"))
changes.Record("offloaded", n, noun(n, "source", "sources"))
```

Evo does **not** own English pluralization (§15). Applications own `noun` helpers.

`Record` exists so the chart can name real domain actions without inventing a method per verb (marble: new domain concept, not sugar symmetry).

---

## Evidence ownership

Evidence attaches **tool-backed proof** (command output tails, etc.) to a Task.
"Stdout" would lie as a name — it also takes stderr and combined writes; Evidence says what
it is for.

- Prefer **Evidence on the Task** (ordinary lead sheet), whether it's a condition or work.
- Prefer **Task.Run(cmd)** for an `*exec.Cmd` — it wires Evidence and Phase in one call.
- Evidence is **silent on success** (PHIL-005).
- Session-level Evidence is studio overdub — not the ordinary example (PHIL-003).

```go
proof := task.Evidence()
```

Who owns the handle: the entity whose condition or work the evidence explains. Do not Capture “somewhere nearby” for convenience.

---

## Aggregate summary Tasks (RULE-002)

Keep a condition Task when it expresses an independent state or carries severity/evidence not represented elsewhere.

**Remove** a condition Task when it only:

- announces that a Plan exists
- repeats successful Changes
- restates a Tasks collection’s derived failure
- says the command succeeded without adding a condition

A **summary Task** may represent the aggregated condition of work intentionally not modeled as a Tasks collection:

```go
placement := out.Task("placement", evo.ID("run.placement"))

if len(summary.Failures) == 0 {
    placement.Done()
} else {
    placement.Fail(fmt.Sprintf("%d failures", len(summary.Failures)), evo.Detail(detailFrom(summary.Failures)))
}
```

Vanity summary Tasks are rejected. Summary Tasks that carry real severity are accepted.

---

## RULE-003 — User-actionable failure cannot be slog-only

`slog` carries implementation diagnostics.

Task Problems carry user-facing failure meaning.

If the user must act or understand a failure, it **must** appear as presentation state — not only as a log line.

---

## RULE-004 — Predeclare concurrent Tasks

Declare Tasks in **deterministic semantic order** before starting workers.

Workers **update** handles; they do not declare presentation order concurrently.

```go
jobs := out.DisplayGroup("placement")
tracked := predeclarePlacementTasks(jobs, sortedFiles)
// then start workers that call tracked[i].Phase / .Bytes / .Done / .Fail
```

---

## RULE-005 — Scale model cardinality to product need

| Workload     | Model                                                     |
| ------------ | --------------------------------------------------------- |
| Small batch  | One predeclared Task per operation                        |
| Medium batch | Aggregate count Task plus selected active large transfers |
| Huge batch   | Aggregate progress plus bounded failures                  |
| Dry-run      | Plan, never simulated Tasks                               |
| Completion   | Changes for durable effects                               |

---

## RULE-006 — Capability does not imply product obligation

A product may remain **summary-only** even when Evo can model per-file Tasks.

Per-file progress is added only when users need confidence during sufficiently long operations. Librarian may stay summary-only.

---

## RULE-007 — Present failure once; propagate errors per app architecture

Both are valid:

```go
task.Fail("tests failed", evo.Cause(err))
return err
```

```go
task.Fail("one expected operation failed", evo.Cause(err))
return nil
```

Evo must not force application error policy. Present the failure for humans; return (or not) according to the app’s error architecture. `Main` reconciles a returned error into Fail only when nothing has failed yet — hosted code mirrors that explicitly.

---

## Domain-correct vs domain-wrong (quick board)

| Call site                                                        | Verdict                                           |
| ---------------------------------------------------------------- | ------------------------------------------------- |
| `out.Task("working tree").Block(..., Detail(...))`               | Correct — condition + user action                 |
| `out.Task("download").Progress` / `.Bytes` / `.Done`             | Correct — work                                    |
| `changes.Record("placed", n, "files")` when domain verb is place | Correct                                           |
| `changes.Added(n, "files placed")`                               | Wrong — generic verb, smuggled domain into object |
| Task that only says “plan ready” next to a Plan section          | Wrong — vanity (RULE-002)                         |
| Summary Task `Fail` over batch failures                          | Correct — aggregate condition                     |
| Failure only in `logger.Error`                                   | Wrong — RULE-003                                  |
| Workers calling `out.Task` concurrently for order                | Wrong — RULE-004                                  |
| Dry-run modeled as Tasks that “succeed” without writing          | Wrong — use Plan (RULE-005)                       |
