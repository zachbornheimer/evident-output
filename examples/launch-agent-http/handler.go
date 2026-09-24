package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

// runHandler serves one launch-agent run per request (spec §53), on the
// 1.1 public API alone: Isolated, FormatExternal, Output.Run, WriteJSON.
//
// Lifecycle: each request gets its own Isolated Output, so concurrent
// requests share no runtime state. The request context — bounded by budget
// — reaches the Tasks unchanged: a disconnected client or an exhausted
// budget ends the running Define, and the run concludes failed with the
// work already committed still reported. The handler, which owns that
// context, is what knows the request ended; it answers 503 then, whatever
// the run concluded. FormatExternal only keeps the run from rendering
// anywhere.
//
// Signals: evo acts on SIGINT/SIGTERM only while a run callback runs.
// This callback only declares the work and returns, so a SIGTERM that
// arrives while a request's Tasks execute is ignored by that run, and the
// server's graceful shutdown lets the request finish.
//
// Backpressure: every request shares stateDir, so runs serialize on its
// exclusive manifest lock (spec §11.3) and each waiting request holds a
// goroutine and a connection. admission bounds that queue: a request
// beyond the limit is turned away at once with 503 and Retry-After, before
// it starts a run. An admitted request's budget covers its wait on the
// lock, and one whose budget runs out in the queue answers 503 at once.
// Nothing is written to the client while its run executes; the document
// is encoded once, after the run, so a slow client delays only its own
// response, never the scheduler.
//
// Errors: the HTTP status comes from the request context and the
// structured Conclusion state, never from message text; the body carries
// the full outcome either way. A failure writing the body is transport
// trouble and is logged, not reported as work failure, under the host's
// own request id (echoed in X-Request-Id), since evo's run_id is out_1 on
// every run.
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
	requestID := requestIDFor(r)
	w.Header().Set(requestIDHeader, requestID)

	ctx, cancel := context.WithTimeout(r.Context(), h.budget)
	defer cancel()

	out := evo.Init(evo.Config{
		Title:    "launch agent",
		StateDir: h.stateDir,
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
		h.log.Warn("launch agent response not delivered", "request_id", requestID, "error", err)
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
// A request whose context ended (client gone, budget spent) is 503 whatever
// the run concluded: the end of ctx failed the running Define, and that is
// not the work's fault.
func statusFor(ctx context.Context, state evo.ConclusionState) int {
	if ctx.Err() != nil {
		return http.StatusServiceUnavailable
	}
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
