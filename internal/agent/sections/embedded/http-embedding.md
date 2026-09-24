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
reports that shape as API-062.

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
  context's values and deadline.
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
- **The cancellation cause** (`by caller` or `deadline exceeded`) is in
  `result.Conclusion.Explanation`. The v2 document records the outcome and
  exit code, not the cause.
