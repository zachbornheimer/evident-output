// Command launch-agent-http proves spec §53: one launch-agent model serves
// both a CLI and an HTTP endpoint. The CLI runs it on the package-default
// Output in any --format; --serve runs it once per request on an Isolated
// FormatExternal Output and answers with the same "evo.run" document
// FormatJSON prints:
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
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

const (
	// defaultBudget bounds one request's run.
	defaultBudget = 30 * time.Second
	// shutdownGrace is how long in-flight requests may finish after SIGTERM.
	shutdownGrace = 10 * time.Second
	// readHeaderTimeout bounds a client's request headers.
	readHeaderTimeout = 5 * time.Second
	// launchPath is the one endpoint --serve exposes.
	launchPath = "POST /launch"
)

type options struct {
	stateDir string
	serve    string
	format   string
	budget   time.Duration
}

func parseOptions() options {
	var o options
	flag.StringVar(&o.stateDir, "state-dir", filepath.Join(os.TempDir(), "evo-launch-agent-http-example"), "manifest and agent state directory")
	flag.StringVar(&o.serve, "serve", "", "listen address; empty runs once as a CLI")
	flag.StringVar(&o.format, "format", "human", "CLI output format: human, json, or jsonl")
	flag.DurationVar(&o.budget, "budget", defaultBudget, "per-request run budget in --serve mode")
	flag.Parse()
	return o
}

func main() {
	o := parseOptions()
	if err := os.MkdirAll(o.stateDir, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "create state dir:", err)
		os.Exit(1)
	}
	a := newAgent(o.stateDir)
	if o.serve == "" {
		os.Exit(runCLI(o, a))
	}
	if err := serve(o, a); err != nil {
		fmt.Fprintln(os.Stderr, "serve:", err)
		os.Exit(1)
	}
}

// runCLI runs the model once on the package-default Output.
func runCLI(o options, a agent) int {
	format, err := evo.ParseFormat(o.format)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	evo.Init(evo.Config{Title: "launch agent", Format: format, StateDir: o.stateDir})
	return evo.Main(func(context.Context) error {
		launchAgent(evo.Default(), a)
		return nil
	})
}

// serve answers POST /launch until SIGINT/SIGTERM, then lets in-flight runs
// finish — the server owns process signals, not the per-request Outputs.
func serve(o options, a agent) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	mux := http.NewServeMux()
	mux.Handle(launchPath, runHandler{agent: a, stateDir: o.stateDir, budget: o.budget, log: slog.Default()})
	srv := &http.Server{Addr: o.serve, Handler: mux, ReadHeaderTimeout: readHeaderTimeout}

	listenErr := make(chan error, 1)
	go func() { listenErr <- srv.ListenAndServe() }()
	select {
	case err := <-listenErr:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := <-listenErr; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
