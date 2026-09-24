// Package main compiles docs/guides/http-embedding.md's handler fence
// verbatim (see TestDocFencesMatchFixtures in the parent package). This is
// a documentation fixture only: go build proves the fence type-checks
// against the shipped API; nothing here is executed. The working, tested
// embedder is examples/launch-agent-http.
package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

// agent, launchAgent, and statusFor are the reader's own model and status
// policy, which the doc fence assumes are in scope.
type agent struct{}

type runHandler struct {
	agent  agent
	budget time.Duration
	log    *slog.Logger
}

func launchAgent(out *evo.Output, _ agent) {
	out.Task("load agent").Define(func(context.Context) error { return nil })
}

func statusFor(context.Context, evo.ConclusionState) int { return http.StatusOK }

// docexamples:snippet start
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

// docexamples:snippet end

func main() {
	http.Handle("/launch", runHandler{log: slog.Default()})
}
