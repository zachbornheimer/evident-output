package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

// runHandler serves one launch-agent run per request (spec §53).
//
// Lifecycle: each request gets its own Isolated, Embedded Output, so
// concurrent requests share no runtime state and the server, not Evo, owns
// process signals. The request context — bounded by budget — is the run's
// only cancellation: a disconnected client or an exhausted budget concludes
// the run Cancelled, with the work already committed still reported.
//
// Backpressure: nothing is written to the client while the run executes;
// the run's human rendering goes to io.Discard. The document is encoded
// once, after the run, so a slow client can delay only its own response,
// never the scheduler. Every request shares stateDir, so requests queue on
// its exclusive manifest lock (spec §11.3) and run one at a time; the
// budget covers that wait, and a request whose budget runs out in the
// queue answers Cancelled at once.
//
// Errors: the HTTP status comes from the structured Conclusion state, never
// from message text; the body carries the full outcome either way. A
// failure writing the body is transport trouble and is logged, not
// reported as work failure.
type runHandler struct {
	agent    agent
	stateDir string
	budget   time.Duration
	log      *slog.Logger
}

func (h runHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.budget)
	defer cancel()

	out := evo.Init(evo.Config{
		Title:    "launch agent",
		StateDir: h.stateDir,
		Isolated: true,
		Embedded: true,
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
		h.log.Warn("launch agent response not delivered", "run_id", result.Conclusion.RunID, "error", err)
	}
}

// statusFor maps a run's outcome to the HTTP status this service promises.
func statusFor(state evo.ConclusionState) int {
	switch state {
	case evo.StateBlocked:
		return http.StatusConflict
	case evo.StateFailed:
		return http.StatusInternalServerError
	case evo.StateCancelled:
		return http.StatusServiceUnavailable
	default:
		return http.StatusOK
	}
}
