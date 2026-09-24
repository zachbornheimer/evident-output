# Decision: An embedded run's caller context end concludes cancelled

**Status:** Accepted
**Date:** 2026-09-23
**IDs:** DEC-CANCEL-001 … DEC-CANCEL-006
**Ticket:** ZYS-946 (spec §53)
**Implementation:** `internal/engine/run.go` (`runInterruptible`),
`internal/engine/run_interruption.go` (`scopeCaller`, `watchCaller`,
`callerScope`, `settledLocked`)

## Context

Through 1.1, the `ctx` passed to `Run`/`Output.Run` reached Define/Verify
callbacks directly. When it ended, a running callback saw `ctx.Done()`,
returned `context.Canceled`, and the run concluded `failed` (exit 2) with a
`context canceled` problem. Queued Tasks kept running. For an HTTP
embedder, a disconnected client therefore read as a server failure, and
work kept running for a request nobody was waiting on.

## Decision

### DEC-CANCEL-001: The caller's ctx is an interrupt, not a failure

When the caller's `ctx` ends, the run stops the same way ^C stops a CLI:
running Tasks are marked cancelled, queued Tasks resolve `not_started`, and
the Conclusion is `cancelled` with `ExitCancelled` (130).
`Conclusion.Explanation` names the cause: `by caller` or
`deadline exceeded`. This applies to `FormatExternal` Outputs only; see
DEC-CANCEL-005.

### DEC-CANCEL-002: One ordered interrupt

The run's own context does not inherit the caller's cancellation. The end
of the caller's `ctx` goes through the same `interrupt` a signal uses:
close the scheduler, mark the rows, then cancel the run context. A Define
cannot observe `Done` and fail its row before the interrupt marks it
cancelled.

### DEC-CANCEL-003: The run context exists before anything can interrupt it

`runInterruptible` installs the run context before it watches signals or
the caller's `ctx`. A `ctx` that had already ended when `Run` was called
interrupts at once; that interrupt must cancel the context the run body
waits on, never a placeholder the run replaces afterwards.

### DEC-CANCEL-004: A cancel that lands after the work is done changes nothing

If the run callback returned while the caller's `ctx` was still live and
every Task is terminal, an interrupt is a no-op: the completed run keeps
its own verdict. A `ctx` already ended when the callback returned still
concludes cancelled, even with no Task left running. The check and the
record of "callback returned" happen under the Output's lock, so the
answer does not depend on goroutine scheduling.

### DEC-CANCEL-005: 1.2 scopes the change to `FormatExternal`

Applied to every caller, DEC-CANCEL-001 is a breaking behavior change: a
`Run`/`Output.Run` caller whose `ctx` ends would get exit 130 instead of
2, and its queued Tasks would stop running. Semver forbids that in a minor
release. So 1.2 applies DEC-CANCEL-001 … 004 only to an Output configured
with `Format: FormatExternal` — the embedding format spec §53 introduces,
whose callers have no 1.1 cancellation behavior to depend on. Every other
format keeps the 1.1 contract exactly: Tasks receive the caller's `ctx`
unchanged, and a Define that returns `ctx.Err()` fails its row (exit 2).

Widening the rule to every format is a separate, breaking decision for a
major release. It needs the maintainer's explicit sign-off; nothing in 1.2
depends on it. The scope lives in one place: `Output.scopeCaller` and
`Output.watchCaller` branch on the `embedded` config bit that
`externalProjection` sets.

### DEC-CANCEL-006: An embedded Task sees no caller deadline

`callerScope` carries the caller's values but reports no deadline.
Deadline-aware callees (a `net.Dialer` derives its connection deadline
from `ctx.Deadline()`) would otherwise time out at the same instant the
deadline's interrupt fires, and could fail their row before the interrupt
marks it cancelled — the race DEC-CANCEL-002 exists to close. The run
context is cancelled with a cause, so `context.Cause(taskCtx)` is
`context.DeadlineExceeded` when the deadline stopped the run, while
`taskCtx.Err()` is `context.Canceled`. A Task that needs its own budget
sets one inside its Define. Pinned by
`TestOutputRun_ExternalHidesCallerDeadlineFromTasks`.

## Consequences

- A `FormatExternal` caller that relied on exit 2 for a cancelled `ctx`
  must branch on `StateCancelled` / exit 130 instead. See
  [`docs/migration/1.2.md`](../migration/1.2.md). Other formats are
  unchanged (`TestOutputRun_NonExternalCallerCancelKeeps11Verdict`).
- `Result.Err` still carries whatever the run callback returned, so an
  embedder can tell work failure from cancellation.
