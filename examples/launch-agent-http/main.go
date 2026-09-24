// Command launch-agent-http proves spec §53: one launch-agent model serves
// both a CLI and an HTTP endpoint. Run once, it declares the model on the
// package-default Output in any --format. With --serve, the process's own
// run is the server: evo.Main owns SIGINT/SIGTERM and turns them into a
// graceful shutdown, while each request runs the model on its own Isolated,
// Embedded Output and answers with the "evo.run" document FormatJSON
// prints:
//
//	go run ./examples/launch-agent-http --format json
//	go run ./examples/launch-agent-http --serve 127.0.0.1:8080
//	curl -X POST 127.0.0.1:8080/launch
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

const (
	// defaultBudget bounds one request's run.
	defaultBudget = 30 * time.Second
	// defaultMaxRequests bounds the requests running or queued at once.
	defaultMaxRequests = 16
	// shutdownGrace is how long in-flight requests may finish after SIGTERM.
	shutdownGrace = 10 * time.Second
	// readHeaderTimeout bounds a client's request headers.
	readHeaderTimeout = 5 * time.Second
	// launchPath is the one endpoint --serve exposes.
	launchPath = "POST /launch"
	// stateDirMode keeps the manifest and agent state private.
	stateDirMode = 0o700
)

type options struct {
	stateDir    string
	serve       string
	format      evo.Format
	budget      time.Duration
	maxRequests int
}

func parseOptions() options {
	o := options{format: evo.FormatHuman}
	flag.StringVar(&o.stateDir, "state-dir", filepath.Join(os.TempDir(), "evo-launch-agent-http-example"), "manifest and agent state directory")
	flag.StringVar(&o.serve, "serve", "", "listen address; empty runs the model once")
	flag.Func("format", "output format: human, json, or jsonl", func(s string) (err error) {
		o.format, err = evo.ParseFormat(s)
		return err
	})
	flag.DurationVar(&o.budget, "budget", defaultBudget, "per-request run budget in --serve mode")
	o.maxRequests = defaultMaxRequests
	flag.Func("max-requests", fmt.Sprintf("requests running or queued at once in --serve mode; more answer 503 (default %d)", defaultMaxRequests), func(s string) error {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			return fmt.Errorf("want a whole number of at least 1, got %q", s)
		}
		o.maxRequests = n
		return nil
	})
	flag.Parse()
	return o
}

func main() {
	o := parseOptions()
	evo.Init(evo.Config{Title: "launch agent", Format: o.format, StateDir: o.stateDir})
	os.Exit(evo.Main(func(context.Context) error {
		if err := os.MkdirAll(o.stateDir, stateDirMode); err != nil {
			return fmt.Errorf("create state dir %s: %w", o.stateDir, err)
		}
		a := newAgent(o.stateDir)
		if o.serve == "" {
			launchAgent(evo.Default(), a)
			return nil
		}
		evo.Task("serve " + o.serve).Define(func(ctx context.Context) error {
			return serve(ctx, o, a)
		})
		return nil
	}))
}

// serve answers POST /launch until ctx ends — evo.Main cancels it on
// SIGINT/SIGTERM — then lets in-flight requests finish. The per-request
// Outputs register no signal handlers of their own.
func serve(ctx context.Context, o options, a agent) error {
	mux := http.NewServeMux()
	mux.Handle(launchPath, runHandler{
		agent: a, stateDir: o.stateDir, budget: o.budget,
		admission: newAdmission(o.maxRequests), log: slog.Default(),
	})
	srv := &http.Server{Addr: o.serve, Handler: mux, ReadHeaderTimeout: readHeaderTimeout}

	listenErr := make(chan error, 1)
	go func() { listenErr <- srv.ListenAndServe() }()
	select {
	case err := <-listenErr:
		return fmt.Errorf("listen on %s: %w", o.serve, err)
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownGrace)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shut down %s: %w", o.serve, err)
	}
	if err := <-listenErr; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve %s: %w", o.serve, err)
	}
	return nil
}
