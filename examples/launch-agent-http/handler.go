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
// process signals. Embedded makes the request context — bounded by budget —
// the run's only cancellation: a disconnected client or an exhausted budget
// concludes the run Cancelled, with the work already committed still
// reported. FormatExternal only keeps the run from rendering anywhere.
//
// Backpressure: every request shares stateDir, so runs serialize on its
// exclusive manifest lock (spec §11.3) and each waiting request holds a
// goroutine and a connection. admission bounds that queue: a request
// beyond the limit is turned away at once with 503 and Retry-After, before
// it starts a run. An admitted request's budget covers its wait on the
// lock, and one whose budget runs out in the queue answers Cancelled at
// once. Nothing is written to the client while its run executes; the
// document is encoded once, after the run, so a slow client delays only
// its own response, never the scheduler.
//
// Errors: the HTTP status comes from the structured Conclusion state, never
// from message text; the body carries the full outcome either way. A
// failure writing the body is transport trouble and is logged, not
// reported as work failure.
type runHandler struct {
	agent     agent
	stateDir  string
	budget    time.Duration
	admission admission
	log       *slog.Logger
}

// retryAfterSeconds is the Retry-After a turned-away request is given.
const retryAfterSeconds = "1"

func (h runHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.admission.tryEnter() {
		w.Header().Set("Retry-After", retryAfterSeconds)
		http.Error(w, "launch agent: too many requests waiting; retry later", http.StatusServiceUnavailable)
		return
	}
	defer h.admission.leave()

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

// admission bounds how many requests are running or queued on the state
// lock at once.
type admission chan struct{}

// newAdmission admits at most limit requests at a time.
func newAdmission(limit int) admission { return make(admission, limit) }

// tryEnter takes a slot, or reports false at once when none is free.
func (a admission) tryEnter() bool {
	select {
	case a <- struct{}{}:
		return true
	default:
		return false
	}
}

// leave frees the slot tryEnter took.
func (a admission) leave() { <-a }

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
