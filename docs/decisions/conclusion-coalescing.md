# Decision: Conclusion coalescing (human projection)

**Status:** Accepted  
**Date:** 2026-07-28  
**IDs:** DEC-COAL-001 … DEC-COAL-005, OPEN-001, OPEN-002  
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

| Visible section | Conclusion state | Suppress? |
|-----------------|------------------|-----------|
| one `[changed]` | `changed` / ready+Changed / unchanged | Yes |
| one `[planned]` | `planned` / ready / unchanged | Yes |
| one section | `failed` / `blocked` / `warning` / `cancelled` / `partial` | No |
| multiple sections | any | No |
| section + failed Item | any (usually failed) | No |
| section + NextCommand | any | No |

## Consequences

- Human output is quieter for summary-only Plan/Changes tools (e.g. librarian).
- Tests that expected a duplicated conclusion band must update.
- Structured consumers are unchanged.

## Tests

`coalesce_test.go`, updated `appendix_h_test.go` Changes goldens.
