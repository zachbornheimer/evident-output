# Domain vocabulary

Binding nouns and verbs for Evident Output call sites.
Source: `docs/roadmap/implementation-basis.md` §5, §8 (RULE-001…007), §4 (PHIL-005).

Cross-links: [jazz-syntax.md](./jazz-syntax.md) · [presentation-boundary.md](./presentation-boundary.md)

---

## Task / Sequence / Group

One leaf entity, one constructor, plus two structural containers. A `Task` answers
both questions "is this state acceptable?" and "how is this work going?" —
which one depends on how it's used, not on a separate type:

| Noun         | Meaning                                                                                                                                                        |
| ------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Task**     | A named condition or unit of work — its check or work runs in `Define`; `Warn`/`Block`/`Fail`/`Skipped` state a condition, `Doing`/`Progress` narrate **work** |
| **Sequence** | Ordered children — each depends on its predecessor; a failed child marks later children `NotStarted`, never a false Done/Pending                               |
| **Group**    | Independent collection — no ordering semantics; any number of children may be `Running` at once                                                                |

Both containers derive their state entirely from their children — never
`.Fail()` or a success stamp on the container itself (see RULE-002 below).

```go
gate := out.Task("working tree") // condition: checked in Define below
work := out.Task("download")     // work: driven through Doing/Progress
packages := out.Group("packages")
```

(Shipped v0.2.x code spelled the condition shape `Item` — folded into `Task`:
one entity, one constructor. `ItemHandle` no longer exists; use `TaskHandle`.)

---

## Warn / Block / Fail

Severity on conditions and terminal outcomes on work — the same verbs either
way. Success is not a verb the caller calls: a `Define` callback that returns
`nil` is the Task holding (1.1 removed `Done`; `Summary` carries optional
result text).

| Outcome                   | User meaning                                          |
| ------------------------- | ----------------------------------------------------- |
| **Define returns nil**    | Condition holds; work succeeded                       |
| **Warn**                  | Proceed, but notice this                              |
| **Block**                 | Stop until the user acts (not necessarily a Go error) |
| **Fail** / returned error | Operation failed                                      |

```go
gate.Define(func(ctx context.Context) error {
    status, err := inspectWorkingTree(ctx)
    if err != nil {
        return fmt.Errorf("could not inspect working tree: %w", err)
    }
    if status.Ignored > 0 {
        gate.Warn("contains ignored files", evo.Detail("2 files"))
    }
    if status.Dirty {
        gate.Block("contains local changes", evo.Detail("stash or commit them"))
    }
    return nil
})
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
task.Block("contains local changes", evo.Detail("stash or commit them"))
return task.Failf("download failed: %w", err)

// Wrong — user message only in the wrapped error, empty human summary
return task.Failf(": %w", err)
```

`evo.Cause` (a `ProblemOption` from before this split existed) is removed: `Fail`/`Block` are
statement-form, so a wrapped error's diagnostic text flows through `Failf`'s trailing `%w`
instead.

---

## Mutation verbs: planned vs changed

| Tense             | When                                       |
| ----------------- | ------------------------------------------ |
| Future / intended | Dry-run, proposed effects, not yet durable |
| Past / durable    | Effects that happened (or were committed)  |

The caller reports the effect and runs its own callback; evo derives the ledger entry, tense,
and `[planned]`/`[changed]` band from `Config.DryRun` — one call-site spelling, never a
tense flip:

```go
task.Define(func(ctx context.Context) error {
	return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectDelete, Object: "package", Quantity: len(ids)}, removePackages)
})
```

RULE-005: dry-run picks `[planned]` from `Config.DryRun`, never a simulated Task. Live picks
`[changed]` the same way, from the same call site.

---

## RULE-001 — Domain verbs over generic verbs

The `EffectVerb` set is closed (`EffectAdd`/`EffectCreate`/`EffectDelete`/`EffectInstall`/
`EffectPush`/`EffectRemove`/`EffectUninstall`/`EffectUpdate`); there is no free-text verb.
Pick the verb that is true of what happened, and put the domain noun in `Object` — never
smuggle the verb into the object.

```go
// Wrong — the verb is smuggled into the object
evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectAdd, Object: "file placed", Quantity: n}, place)

// Right — the object is the final grammatical object
evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectCreate, Object: "file", Quantity: n}, place)
```

Evo pluralizes `Object` from `Quantity`. A classification that changes nothing (`ready`,
`reused`, `blocked`) is information, not an Effect: `task.Fact("reused", "63 packages")`.
`Record`/`RecordLabel`/`RecordName` were removed in 1.1 (ZYS-974): reporting a mutation after
it already happened bypasses dry-run planning, so the mutation itself moves into `Effect`.

---

## Evidence ownership

Evidence attaches **tool-backed proof** (command output tails, etc.) to a Task.
"Stdout" would lie as a name — it also takes stderr and combined writes; Evidence says what
it is for.

- Prefer **Writer on the Task** (ordinary lead sheet), whether it's a condition or work.
- `cmd.Stdout = task.Writer()` (and stderr) wires child chatter into the live doing-text.
- Retained evidence is **silent on success** (PHIL-005).

```go
cmd.Stdout = task.Writer()
cmd.Stderr = task.Writer()
```

Who owns the handle: the entity whose condition or work the evidence explains. Do not Capture “somewhere nearby” for convenience.

---

## Aggregate summary Tasks (RULE-002)

Keep a condition Task when it expresses an independent state or carries severity/evidence not represented elsewhere.

**Remove** a condition Task when it only:

- announces that a Plan exists
- repeats successful Changes
- restates a Sequence/Group’s derived failure
- says the command succeeded without adding a condition

A **summary Task** may represent the aggregated condition of work intentionally not modeled as a Sequence/Group:

```go
placement := out.Task("placement")
placement.Define(func(ctx context.Context) error {
    summary := place(ctx)
    if len(summary.Failures) > 0 {
        placement.Fail(fmt.Sprintf("%d failures", len(summary.Failures)), evo.Detail(detailFrom(summary.Failures)))
    }
    return nil
})
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
jobs := out.Group("placement")
tracked := predeclarePlacementTasks(jobs, sortedFiles)
// then tracked[i].Define(...) submits each item; its callback drives .Doing / .Bytes / .Fail
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
task.Failf("tests failed: %w", err)
return err
```

```go
task.Failf("one expected operation failed: %w", err)
return nil
```

Evo must not force application error policy. Present the failure for humans; return (or not) according to the app’s error architecture. `Main` reconciles a returned error into Fail only when nothing has failed yet — hosted code mirrors that explicitly.

---

## Domain-correct vs domain-wrong (quick board)

| Call site                                                    | Verdict                                           |
| ------------------------------------------------------------ | ------------------------------------------------- |
| `out.Task("working tree").Block(..., Detail(...))`           | Correct — condition + user action                 |
| `out.Task("download").Define` driving `.Progress` / `.Bytes` | Correct — work                                    |
| `Effect{Verb: EffectCreate, Object: "file", Quantity: n}`    | Correct                                           |
| `Effect{Verb: EffectAdd, Object: "files placed"}`            | Wrong — generic verb, smuggled domain into object |
| Task that only says “plan ready” next to a Plan section      | Wrong — vanity (RULE-002)                         |
| Summary Task `Fail` over batch failures                      | Correct — aggregate condition                     |
| Failure only in `logger.Error`                               | Wrong — RULE-003                                  |
| Workers calling `out.Task` concurrently for order            | Wrong — RULE-004                                  |
| Dry-run modeled as Tasks that “succeed” without writing      | Wrong — use Plan (RULE-005)                       |
