# Domain vocabulary

Binding nouns and verbs for Evident Output call sites.
Source: `docs/roadmap/implementation-basis.md` §5, §8 (RULE-001…007), §4 (PHIL-005).

Cross-links: [jazz-syntax.md](./jazz-syntax.md) · [presentation-boundary.md](./presentation-boundary.md)

---

## Item / Task / Tasks

| Noun      | Meaning                                                                         |
| --------- | ------------------------------------------------------------------------------- |
| **Item**  | An independent **condition** — something that is OK, warned, blocked, or failed |
| **Task**  | A unit of **work** — phases, progress, done/fail                                |
| **Tasks** | A **collection** of Tasks; collection state is **derived** from children        |

```go
item := out.Item("working tree", evo.ID("repo.working-tree"))
task := out.Task("download", evo.ID("install.download"))
packages := out.Tasks("packages")
```

**Test:** If it answers “is this state acceptable?” → Item. If it answers “how is this work going?” → Task.

---

## Warn / Block / Fail

Severity on conditions (Items) and terminal outcomes on work (Tasks):

| Verb      | User meaning                                          |
| --------- | ----------------------------------------------------- |
| **OK**    | Condition holds; work succeeded                       |
| **Warn**  | Proceed, but notice this                              |
| **Block** | Stop until the user acts (not necessarily a Go error) |
| **Fail**  | Operation failed                                      |

```go
item.OK()
item.Warn("contains ignored files", evo.Detail("2 files"))
item.Block("contains local changes", evo.Detail("stash or commit them"))
item.Fail("could not inspect working tree", evo.Cause(err))
```

Plural structured evidence only when evidence is actually plural:

```go
branches.BlockedBy(problems...)
item.FailedBy(problemsFrom(summary.Failures)...)
```

`BlockedBy` / `FailedBy` / `WarnedBy` are genuine plural voicing (PHIL-002) — not sugar for the singular form.

---

## Problem / Detail / Cause

| Piece       | Audience            | Role                                                              |
| ----------- | ------------------- | ----------------------------------------------------------------- |
| **Problem** | Structured evidence | Subject + summary (+ optional pieces) for one failure unit        |
| **Detail**  | **User-facing**     | What the human should know or do                                  |
| **Cause**   | **Diagnostic**      | Underlying error for logs / debug; not the primary human sentence |

PHIL-005: `Cause` is diagnostic; `Detail` is user-facing. Do not put stack-trace noise in Detail or bury the only user message in Cause alone.

```go
// Right
item.Block("contains local changes", evo.Detail("stash or commit them"))
task.Fail("download failed", evo.Cause(err))

// Wrong — user message only in Cause, empty human summary
task.Fail("", evo.Cause(err))
```

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

## Capture ownership

Capture attaches **tool-backed evidence** (command output tails, etc.) to an Item or Task.

- Prefer **Capture on Item or Task** (ordinary lead sheet).
- Capture is **silent on success** (PHIL-005).
- Session-level Capture is studio overdub — not the ordinary example (PHIL-003).

```go
captured := item.Capture()
captured := task.Capture()
```

Who owns the handle: the entity whose condition or work the evidence explains. Do not Capture “somewhere nearby” for convenience.

---

## Aggregate summary Items (RULE-002)

Keep an Item when it expresses an independent condition or carries severity/evidence not represented elsewhere.

**Remove** an Item when it only:

- announces that a Plan exists
- repeats successful Changes
- restates a Tasks collection’s derived failure
- says the command succeeded without adding a condition

A **summary Item** may represent the aggregated condition of work intentionally not modeled as Tasks:

```go
placement := out.Item("placement", evo.ID("run.placement"))

if len(summary.Failures) == 0 {
    placement.OK()
} else {
    placement.FailedBy(problemsFrom(summary.Failures)...)
}
```

Vanity Items are rejected. Summary Items that carry real severity are accepted.

---

## RULE-003 — User-actionable failure cannot be slog-only

`slog` carries implementation diagnostics.

Item/Task Problems carry user-facing failure meaning.

If the user must act or understand a failure, it **must** appear as presentation state — not only as a log line.

---

## RULE-004 — Predeclare concurrent Tasks

Declare Tasks in **deterministic semantic order** before starting workers.

Workers **update** handles; they do not declare presentation order concurrently.

```go
jobs := out.Tasks("placement")
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
| `out.Item("working tree").Block(..., Detail(...))`               | Correct — condition + user action                 |
| `out.Task("download").Progress` / `.Bytes` / `.Done`             | Correct — work                                    |
| `changes.Record("placed", n, "files")` when domain verb is place | Correct                                           |
| `changes.Added(n, "files placed")`                               | Wrong — generic verb, smuggled domain into object |
| Item that only says “plan ready” next to a Plan section          | Wrong — vanity (RULE-002)                         |
| Summary Item `FailedBy` over batch failures                      | Correct — aggregate condition                     |
| Failure only in `logger.Error`                                   | Wrong — RULE-003                                  |
| Workers calling `out.Task` concurrently for order                | Wrong — RULE-004                                  |
| Dry-run modeled as Tasks that “succeed” without writing          | Wrong — use Plan (RULE-005)                       |
