# Embedding Evo behind HTTP

Spec §53: the CLI and an HTTP endpoint run the same model. One function
declares the work. The CLI runs it on the package default Output in any
`--format`. An HTTP handler runs it on a fresh Isolated Output per request
and answers with `evo.WriteJSON` — the same `"evo.run"` document
`FormatJSON` prints, byte for byte, for the same run.

Everything here is the 1.1 public API; 1.2 adds none. `examples/launch-agent-http`
is the working embedder, and its tests prove every rule below. A
host-owned lifecycle (the request context as the run's interrupt, a
per-run `run_id`) needs new public surface and waits for a second real
consumer (ZYS-947; see
[DEC-CANCEL-005](../decisions/caller-cancellation.md#dec-cancel-005-no-lifecycle-change-and-no-new-public-surface-in-12-accepted)).

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
	w.WriteHeader(statusFor(ctx, result.Conclusion.State))
	if err := evo.WriteJSON(w, result); err != nil {
		h.log.Warn("response not delivered", "error", err)
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
reports that shape as API-062, including inside the model functions the
handler calls. Spec §53's sample writes
`launchAgent(ctx, agent)`; the `*evo.Output` parameter is the recorded
deviation that makes it work
([`decisions/http-embedding-declaration-target.md`](../decisions/http-embedding-declaration-target.md)).

`evo.File`, `evo.Effect`, and `evo.Exec` take the Define callback's `ctx`,
so they already land on the right Output.

## Lifecycle

- **One Output per request.** Isolated Outputs share no runtime state.
  Every run still carries the 1.1 `run_id`, `out_1`, so correlate
  requests on your own request id, not on `run_id`.
- **The request context reaches the Tasks unchanged.** When it ends — the
  client disconnects or the handler's budget runs out — the running Define
  sees `ctx.Done()`, fails its row, and the run concludes `failed`
  (exit 2); the Tasks after it in a `Sequence` are `not_started`. Work
  already committed stays in the document's `effects`. The handler owns
  that context, so it is the one that knows the request ended: check
  `ctx.Err()` before mapping the Conclusion to a status.
- **Signals.** evo acts on SIGINT/SIGTERM only while a run callback runs;
  a signal that arrives after it returned is caught and ignored. A handler
  callback that only declares the work returns at once, so a server's
  graceful SIGTERM lets in-flight requests finish. The server's own run
  (`evo.Main`) must serve inside its callback, not in a Define, for ^C to
  shut it down.
- **`FormatExternal` only keeps the run from rendering anywhere.**
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
runs out in the queue, the request answers right away: the waiting Task
fails with the deadline, the Tasks after it are `not_started`, and the
handler answers 503 because its `ctx` ended. Size the budget for the wait as well as the work, or give
independent workspaces their own `StateDir`.

Bound the queue. Every waiting request holds a goroutine and a connection
until its budget runs out, and evo sets no limit on how many wait. Admit a
fixed number of requests at a time and turn the rest away at once with
`503` and `Retry-After`, before they start a run.
`examples/launch-agent-http` does this with a buffered channel
(`--max-requests`, default 16); a turned-away request gets a plain-text
body, not an `"evo.run"` document, because it ran nothing.

Streaming `FormatJSONL` over HTTP is not a supported projection: each
event line is written synchronously as it happens, so a slow reader would
throttle the run itself.

## Errors and status

- **Status comes from the request context and structured state.** If
  `ctx.Err()` is non-nil, the request ended (client gone, budget spent):
  answer 503 whatever the run concluded. Otherwise map
  `result.Conclusion.State` (`ready`, `blocked`, `failed`, `cancelled`) to
  your status codes. Never parse message text. The body carries the full
  outcome either way.
- **Work failure is not transport failure.** `result.Err` is the error your
  run function returned. A `WriteJSON` write error is the writer's own
  error, unchanged from 1.1, so compare it as you would any transport
  error. Log it; the run's truth is unchanged.
- **A cancelled document names its cause.** A run stopped by SIGINT/SIGTERM
  carries `"cancellation": {"cause": "user"}`; the same words are in
  `result.Conclusion.Explanation` (`by user`). The field is absent on any
  run that did not conclude cancelled
  ([DEC-CANCEL-007](../decisions/caller-cancellation.md#dec-cancel-007-the-wire-document-names-the-cancellation-cause)).
