# Decision: Conclusion coalescing (human projection)

**Status:** Accepted
**Date:** 2026-07-28
**IDs:** DEC-COAL-001 … DEC-COAL-008, OPEN-001, OPEN-002
**Implementation:** `coalesce.go` (`shouldSuppressStandaloneConclusion`)

## Context

When a command has exactly one Plan or Changes section whose subject matches the
output title, the human renderer previously printed the section header and then
repeated the same tag+subject as a trailing conclusion band:

```text
[changed]  librarian
  placed     1 file

[changed]  librarian
```

That is redundant chrome, not additional meaning.

## Decision

### DEC-COAL-001 — Projection only

`Conclusion` and Plan/Changes remain independent in the model, snapshots, and
JSON. Coalescing only skips `writeConclusion` in human residual/plain render paths.

### DEC-COAL-002 — Semantic identity

Subject match uses normalized display subjects (`TrimSpace` + lower) against:

1. effect section subject
2. conclusion subject
3. output subject (Config.Title)

Optional: section ID equals conclusion/output subject when set.

Display-string-only matching is insufficient for plugins with identical labels and
different IDs — when IDs diverge from titles, both sides must align.

### DEC-COAL-003 — Narrow suppression rule

Suppress the trailing conclusion band only when **all** hold:

1. exactly one visible Plan **or** Changes section (not both, not zero, not many)
2. section subject/ID matches conclusion/output subject
3. conclusion state is compatible (see matrix)
4. conclusion has no Explanation, Actions, Partial, or Cancelled flags
5. no independent condition rows (Item/Task/Collection in Failed/Blocked/Warning/in-flight)

### DEC-COAL-004 — Preserve when dimension is added

Do **not** suppress for failed, blocked, warning, cancelled, partial completion,
multiple effect sections, Plan+Changes together, NextCommand/actions, subject mismatch.

### DEC-COAL-005 — Compatibility matrix

| Visible section       | Conclusion state                                           | Suppress? |
| --------------------- | ---------------------------------------------------------- | --------- |
| one `[changed]`       | `changed` / ready+Changed / unchanged                      | Yes       |
| one `[planned]`       | `planned` / ready / unchanged                              | Yes       |
| one section           | `failed` / `blocked` / `warning` / `cancelled` / `partial` | No        |
| multiple sections     | any                                                        | No        |
| section + failed Item | any (usually failed)                                       | No        |
| section + NextCommand | any                                                        | No        |

### DEC-COAL-006 — Evidence, not titles, establishes readiness

A non-empty output title does not establish a successful result. `ready` requires
at least one resolved Item, Task, or Tasks collection; an output with only a title
or ordinary lines concludes `unchanged`.

### DEC-COAL-007 — Suppress conclusions with no information gain

The human projection omits a conclusion when either:

1. no semantic result exists and the conclusion has no explanation, action, partial,
   or cancellation dimension; or
2. exactly one Item, Task, or Tasks collection already displays the same state and
   semantic subject.

The structured model retains the conclusion in both cases.

### DEC-COAL-008 — Preserve subject-level rollups

A single condition may still justify a conclusion when its subject differs from
the output subject. For example, a failed `release signature` condition may add the
broader conclusion `[blocked] release v1.4`. Multiple condition results also retain
their aggregate conclusion.

## Consequences

- Human output is quieter for summary-only Plan/Changes tools (e.g. librarian).
- Data-only commands no longer gain conclusion chrome from `Title` alone.
- Exact one-condition restatements collapse without weakening structured output.
- Tests that expected a duplicated conclusion band must update.
- Structured consumers are unchanged.

## Tests

`coalesce_test.go`, updated `appendix_h_test.go` Changes goldens.
