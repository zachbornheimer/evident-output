# Decision: 1.2 keeps the 1.1 run lifecycle; a caller-owned one waits for a real consumer

**Status:** Accepted
**Date:** 2026-09-23 (DEC-CANCEL-001 … 004 and 006 drafted);
2026-09-24 (DEC-CANCEL-005 rewritten under the maintainer's rules below,
DEC-CANCEL-007 accepted)
**IDs:** DEC-CANCEL-001 … DEC-CANCEL-007
**Ticket:** ZYS-946 (spec §53); deferred work blocked by ZYS-947
**Implementation:** `internal/engine/run.go` (`runInterruptible`, the 1.1
signal window), `internal/engine/run_interruption.go` (`interrupt`),
`internal/engine/manifest_open.go` (the state-lock wait),
`internal/engine/wireformat.go` (`WriteRunDocument`),
`internal/wire/run.go` (`cancellationFor`)

## Context

Through 1.1, the `ctx` passed to `Run`/`Output.Run` reached Define/Verify
callbacks directly. When it ended, a running callback saw `ctx.Done()`,
returned `context.Canceled`, and the run concluded `failed` (exit 2) with a
`context canceled` problem. Queued Tasks kept running. evo acted on
SIGINT/SIGTERM only while the run callback ran; a signal after it returned
was caught and ignored. For an HTTP embedder, a disconnected client
therefore reads as a server failure, and every run carries `run_id`
`out_1`.

## Decision

### DEC-CANCEL-005: No lifecycle change and no new public surface in 1.2 (Accepted)

**Rules (maintainer, 2026-09-24), binding:**

1. A minor release changes no behavior for an existing 1.1 host, CLI
   formats included. A new lifecycle is opt-in only.
2. Error identity is behavior. Every 1.1 error path returns the error it
   returned in 1.1.
3. ZYS-946 requires one real embedder before the public surface grows.
   1.2 adds no exported API; anything that needs it waits for the next
   real consumer, ZYS-947.

So 1.2 ships:

- **The 1.1 signal window on every format.** An earlier draft of this
  branch watched signals until the run concluded, so a ^C during Finish
  stopped the run on the formats evo renders. That changed 1.1 behavior
  and is reverted: human, `FormatData`, `FormatJSON`, `FormatJSONL`, and
  `FormatExternal` all act on a signal only while the run callback runs.
  Pinned by `TestRun_SignalAfterCallbackIsIgnoredOnEveryFormat` and
  `TestRun_SignalDuringCallbackStopsTheRun`.
- **The 1.1 caller `ctx`.** It reaches Tasks unchanged, and its end fails
  the running Define (exit 2). Pinned by
  `TestOutputRun_CallerCancelKeeps11Verdict` and
  `TestOutputRun_CallerDeadlineReachesTasks`.
- **The 1.1 run identity**, `out_1`, with the first Task at `task_2`.
  Pinned by `TestRunID_Keeps11IdentityOnEveryProjection` and
  `TestRunDocument_TaskIDsKeepTheirNumbering`.
- **The 1.1 errors.** `WriteJSON` returns the writer's error itself
  (`TestWriteJSON_WriterFailureReturnsTheWriterError`). A `FormatJSON`
  write failure is one wrap of `ErrRenderer` carrying the writer error's
  text, and does not match the writer's error under `errors.Is`
  (`TestFormatJSON_WriterFailureKeeps11Identity`). Both documents come
  from one encoder, `wire.EncodeRunLine`, so their bytes cannot drift.
- **A fix inside the 1.1 contract.** A ^C during the run callback now
  stops a run whose Define is queued on another run's state lock
  (spec §11.3). The wait used to hold the Output's lock, so the interrupt
  hung until the other run finished. Pinned by
  `TestRun_SignalStopsARunQueuedOnTheStateLock`.
- **DEC-CANCEL-007**, below, which is additive.

Deferred behind ZYS-947, because each needs a new exported `Config` field
(an `Embedded` opt-in, a `RunID` pin): DEC-CANCEL-001 … 004 and 006, and a
per-run `run_id`. When ZYS-947 lands a second real consumer, that consumer's
evidence decides the option's shape; the drafts below are the starting
point, not a commitment.

### DEC-CANCEL-001 (deferred): The caller's ctx is an interrupt, not a failure

For an opted-in run, when the caller's `ctx` ends, the run stops the same
way ^C stops a CLI: running Tasks are marked cancelled, queued Tasks
resolve `not_started`, and the Conclusion is `cancelled` with
`ExitCancelled` (130), `Conclusion.Explanation` `by caller` or
`deadline exceeded`.

### DEC-CANCEL-002 (deferred): One ordered interrupt

The run's own context does not inherit the caller's cancellation. The end
of the caller's `ctx` goes through the same `interrupt` a signal uses:
close the scheduler, mark the rows, then cancel the run context, so a
Define cannot observe `Done` and fail its row first.

### DEC-CANCEL-003 (deferred): The run context exists before anything can interrupt it

A `ctx` that had already ended when `Run` was called must cancel the
context the run body waits on, never a placeholder the run replaces
afterwards.

### DEC-CANCEL-004 (deferred): A cancel after the work is done changes nothing

If the run callback returned while the caller's `ctx` was live and every
Task is terminal, a caller interrupt is a no-op. Separately, and shipped
in 1.2: once `Finish` has fixed the Conclusion, no interrupt changes
anything (`Output.interrupt` checks `finished`; pinned by
`TestInterrupt_AfterConclusionIsANoOp`).

### DEC-CANCEL-006 (deferred): An opted-in Task sees no caller deadline

An opted-in run's Tasks would descend from `context.WithoutCancel(ctx)`,
so a deadline-aware callee (`net.Dialer`) cannot time out and fail its row
at the instant the deadline's interrupt fires. `context.Cause` on the Task
`ctx` would report `context.DeadlineExceeded`.

### DEC-CANCEL-007: The wire document names the cancellation cause

An HTTP client reads only the body, and `Conclusion.Explanation` is human
text it must not parse (§53). So the v2 `"evo.run"` document gains an
optional `cancellation` object on a run that SIGINT/SIGTERM cancelled:
`{"cause": "user"}`. The JSONL `run.finished` event carries the same
object in its payload. The field is absent on any other outcome, so every
other document is byte-identical to 1.1, and a v2 reader that ignores
unknown keys is unaffected. `schema_version` stays `2.0`;
`schema/run.v2.json` lists the property with the one cause 1.2 emits.
ZYS-947 adds `caller` and `deadline` with the opt-in lifecycle.

The code travels on `core.Conclusion` as an unexported field
(`core.CancelCauseOf`), so it adds no exported API. `wire.RunFinishedPayload`
builds the JSONL payload from the same Conclusion through the same
mappings as the document, so the two cannot disagree. Pinned by
`TestFormatJSON_SignalledRunNamesUserCause`,
`TestFormatJSON_CompletedRunHasNoCancellation`,
`TestFormatJSONL_RunFinishedNamesUserCause`,
`TestCancellationFor_OnlyOnCancelledRunsWithACause`, and
`TestRunFinishedPayload_AgreesWithTheRunDocument`.

## Consequences

- No 1.1 host changes on upgrade: the same signal window, `ctx` contract,
  exit codes, `run_id`, and errors on every format.
- An HTTP embedder on 1.2 checks its own `ctx.Err()` to tell a request
  that ended from work that failed, and correlates on its own request id
  (`docs/guides/http-embedding.md`, `examples/launch-agent-http`).
- `Result.Err` still carries whatever the run callback returned, so an
  embedder can tell work failure from cancellation.
