# Decision: An embedded run's caller context end concludes cancelled

**Status:** Accepted
**Date:** 2026-09-23 (DEC-CANCEL-005 accepted by the maintainer and
DEC-CANCEL-007 added 2026-09-24)
**IDs:** DEC-CANCEL-001 … DEC-CANCEL-007
**Ticket:** ZYS-946 (spec §53)
**Implementation:** `internal/engine/construct.go` (`Config.Embedded`),
`internal/engine/run.go` (`runInterruptible`),
`internal/engine/run_interruption.go` (`scopeCaller`, `watchCaller`,
`settledLocked`)

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
`deadline exceeded`. This applies only to an Output whose Config sets
`Embedded: true`; see DEC-CANCEL-005.

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

This rule is about a caller's `ctx`, so it applies to embedded runs only
(DEC-CANCEL-005). A CLI run never settles: a ^C that lands after the
callback returned and every Task finished still concludes the run
cancelled (exit 130), because the person at the terminal asked it to
stop. `Output.endRunCallback` gates on the `embedded` bit, and
`TestInterrupt_SignalAfterEveryTaskFinishedStillCancelsCLIRun` pins it.

Once `Finish` has fixed the Conclusion, no interrupt changes anything, on
any format. A signal that lands between the run concluding and `Run`
returning is a no-op, so the Output's own state and the `Result` it
returned always agree (`Output.stopsNothingLocked`, pinned by
`TestRun_SignalAfterConclusionLeavesTheResult`).

### DEC-CANCEL-005: The new lifecycle is opt-in through `Config.Embedded` (Accepted)

**Rule (maintainer, 2026-09-24):** no behavior break for existing 1.1
`FormatExternal` hosts. The caller-cancellation lifecycle is opt-in,
through the smallest additive option consistent with existing idioms. The
default stays 1.1.

Applied to every caller, DEC-CANCEL-001 is a breaking behavior change: a
`Run`/`Output.Run` caller whose `ctx` ends would get exit 130 instead of
2, and its queued Tasks would stop running. An earlier draft scoped it to
`FormatExternal` on the theory that no 1.1 caller depended on that
format's cancellation. That was false: `FormatExternal` was public in 1.1
for host-owned rendering (`FormatExternal` + `out.Snapshot()`), and a 1.1
host rendering in a terminal would have lost evo's SIGINT/SIGTERM handling
and seen 130 where it saw 2.

So 1.2 adds one `Config` bool, `Embedded`, alongside `Isolated`:

- `Embedded: true` applies DEC-CANCEL-001 … 004 and 006: evo registers no
  SIGINT/SIGTERM handler, the end of `ctx` interrupts the run, and Tasks
  see the caller's values without its cancellation or deadline.
- Without it, every run keeps the 1.1 contract exactly, `FormatExternal`
  included: Tasks receive the caller's `ctx` unchanged, a Define that
  returns `ctx.Err()` fails its row (exit 2), and the run owns ^C.

`Embedded` is independent of `Format`. `FormatExternal` chooses how a run
renders; `Embedded` chooses who owns its lifecycle. An HTTP handler
usually sets both (`examples/launch-agent-http`), and a host that streams
the `FormatJSON` document itself can set `Embedded` alone. Like `Isolated`,
it is honored on the `Config.Options` path too.

Widening DEC-CANCEL-001 to every run by default is a separate breaking
decision for a major release. The scope lives in one place:
`Output.scopeCaller`, `Output.watchCaller`, `Output.endRunCallback`, and
`Output.subscribeProcessSignals` branch on the `embedded` config bit, which
only `Config.Embedded` sets. Pinned by
`TestOutputRun_CallerCancelWithoutEmbeddedKeeps11Verdict`,
`TestOutputRun_EmbeddedOptsAnyFormatIntoCallerCancellation`,
`TestRun_EmbeddedLeavesProcessSignalsToHost`, and
`TestRun_FormatExternalWithoutEmbeddedStillOwnsProcessSignals`.

### DEC-CANCEL-006: An embedded Task sees no caller deadline

An embedded run's Tasks descend from `context.WithoutCancel(ctx)`: the
caller's values, with no cancellation and no deadline.
Deadline-aware callees (a `net.Dialer` derives its connection deadline
from `ctx.Deadline()`) would otherwise time out at the same instant the
deadline's interrupt fires, and could fail their row before the interrupt
marks it cancelled — the race DEC-CANCEL-002 exists to close. The run
context is cancelled with a cause, so `context.Cause(taskCtx)` is
`context.DeadlineExceeded` when the deadline stopped the run, while
`taskCtx.Err()` is `context.Canceled`. A Task that needs its own budget
sets one inside its Define. Pinned by
`TestOutputRun_ExternalHidesCallerDeadlineFromTasks`.

### DEC-CANCEL-007: The wire document names the cancellation cause

An HTTP client reads only the body, and `Conclusion.Explanation` is human
text it must not parse (§53). So the v2 `"evo.run"` document gains an
optional `cancellation` object on a cancelled run:
`{"cause": "caller" | "deadline" | "user"}`. The JSONL `run.finished`
event carries the same object in its payload. The field is absent on any
other outcome, so every existing document except a cancelled one is
byte-identical, and a v2 reader that ignores unknown keys is unaffected.
`schema_version` stays `2.0`; `schema/run.v2.json` lists the property.

The code travels on `core.Conclusion` as an unexported field
(`core.CancelCauseOf`). Go embedders already have the cause:
`Conclusion.Explanation`, or the handler's own `ctx.Err()`. Promoting it
to a public `Conclusion` field is a separate API decision.
`wire.RunFinishedPayload` builds the JSONL payload from the same
Conclusion through the same mappings as the document, so the two cannot
disagree. Pinned by `TestOutputRun_CallerContextEndConcludesCancelled`,
`TestFormatJSON_SignalledRunNamesUserCause`,
`TestFormatJSONL_RunFinishedNamesUserCause`,
`TestCancellationFor_OnlyOnCancelledRunsWithACause`, and
`TestRunFinishedPayload_AgreesWithTheRunDocument`.

## Consequences

- No 1.1 caller changes behavior on upgrade. A host that wants the §53
  lifecycle sets `Config.Embedded` (DEC-CANCEL-005), then branches on
  `StateCancelled` / exit 130 and owns SIGINT/SIGTERM itself. See
  [`docs/migration/1.2.md`](../migration/1.2.md).
- `Result.Err` still carries whatever the run callback returned, so an
  embedder can tell work failure from cancellation.
