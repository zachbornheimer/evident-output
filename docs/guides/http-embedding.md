# Embedding Evo behind HTTP

Spec §53: the CLI and an HTTP endpoint run the same model. One function
declares the work. The CLI runs it on the package default Output in any
`--format`. An HTTP handler runs it on a fresh Isolated Output per request
and answers with `evo.WriteJSON` — the same `"evo.run"` document
`FormatJSON` prints, byte for byte, for the same run.

`examples/launch-agent-http` is the working embedder; its tests prove every
rule below.

## The handler

```go
func (h runHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.budget)
	defer cancel()

	out := evo.Init(evo.Config{
		Isolated: true,
		Format:   evo.FormatExternal,
		Stdout:   io.Discard,
		Stderr:   io.Discard,
	})
	result := out.Run(ctx, func(context.Context) error {
		launchAgent(out, h.agent)
		return nil
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusFor(result.Conclusion.State))
	if err := evo.WriteJSON(w, result); err != nil {
		h.log.Warn("response not delivered", "run_id", result.Conclusion.RunID, "error", err)
	}
}
```

## Declare on the Output you run

`launchAgent(out, ...)` takes the `*evo.Output` that drives it. The CLI
passes `evo.Default()`; the handler passes its per-request Output.
Package-level `evo.Task`, `evo.Group`, `evo.Sequence`, `evo.Fact`,
`evo.Warn`, `evo.Print*`, and `evo.Confirm` always reach the package
default, so inside an Isolated run they would put every request's work on
one shared Output and leave the request's own document empty. The MCP
reports that shape as API-062. Spec §53's sample writes
`launchAgent(ctx, agent)`; the `*evo.Output` parameter is the recorded
deviation that makes it work
([`decisions/http-embedding-declaration-target.md`](../decisions/http-embedding-declaration-target.md)).

`evo.File`, `evo.Effect`, and `evo.Exec` take the Define callback's `ctx`,
so they already land on the right Output.

## Lifecycle

- **One Output per request.** Isolated Outputs share no runtime state, and
  each run carries its own random `run_id`.
- **The request context is the only cancellation.** When it ends — the
  client disconnects or the handler's budget runs out — the run stops the
  same way ^C stops a CLI: running Tasks are marked cancelled, queued Tasks
  never start, and the Conclusion is `cancelled` with exit code 130. Work
  already committed stays in the document's `effects`. Tasks see the
  context's values, but not its cancellation or deadline: both reach them
  only through that interrupt, so a deadline-aware call (a `net.Dialer`)
  cannot time out and fail its row first. `context.Cause(ctx)` in a Task
  reports `context.DeadlineExceeded` when the budget ran out. This applies
  to `FormatExternal` only; other formats pass the context through
  unchanged.
- **A cancel after the work is done changes nothing.** If the context ends
  after the run callback returned and every Task finished, the run keeps
  its own verdict.
- **The server owns process signals.** A `FormatExternal` run registers no
  SIGINT/SIGTERM handler, so the server's graceful shutdown lets in-flight
  requests finish. Other formats keep the CLI behavior: the run owns ^C.
- **`Output.Run` finishes and closes the Output.** Build a new one for
  every request.

## Backpressure

Nothing is written to the client while the run executes. With
`FormatExternal`, the run's human rendering goes to `Config.Stdout`; pass
`io.Discard`, never the `ResponseWriter`, because rendering is synchronous
with the run. `WriteJSON` encodes the whole document once, after the run,
on the handler's goroutine — a slow client delays only its own response,
never the scheduler.

Requests that share a `StateDir` queue on its state lock. The first
`evo.File` or `evo.Exec` in a run takes an exclusive lock on the
workspace's manifest (spec §11.3) and holds it until the run ends, so two
requests over the same workspace run one after the other, not side by
side. A queued request's budget keeps running down while it waits: if it
runs out in the queue, the request answers `cancelled` (exit code 130)
right away, with the waiting Task cancelled and the Tasks after it
`not_started`. Size the budget for the wait as well as the work, or give
independent workspaces their own `StateDir`.

Streaming `FormatJSONL` over HTTP is not a supported projection: each
event line is written synchronously as it happens, so a slow reader would
throttle the run itself.

## Errors and status

- **Status comes from structured state.** Map `result.Conclusion.State`
  (`ready`, `blocked`, `failed`, `cancelled`) to your status codes. Never
  parse message text. The body carries the full outcome either way.
- **Work failure is not transport failure.** `result.Err` is the error your
  run function returned. A `WriteJSON` error means the response did not
  reach the client; it names the failed step and wraps the writer's error,
  so `errors.Is` still matches it. Log it; the run's truth is unchanged.
- **The cancellation cause is in the body.** A cancelled document carries
  `"cancellation": {"cause": "caller"}` when the request context was
  cancelled, `"deadline"` when its deadline passed, and `"user"` for ^C on
  a CLI run. An HTTP client tells a server budget timeout from a shutdown
  by that field, not by the status code or the human text. The same words
  are in `result.Conclusion.Explanation` (`by caller`,
  `deadline exceeded`, `by user`). The field is absent on any run that did
  not conclude cancelled
  ([DEC-CANCEL-007](../decisions/caller-cancellation.md#dec-cancel-007-the-wire-document-names-the-cancellation-cause)).
