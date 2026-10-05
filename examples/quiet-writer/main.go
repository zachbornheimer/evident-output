// Command quiet-writer demos a Task that writes bounded Writer evidence, then
// stays Running with no further application output before it settles.
//
//	go run ./examples/quiet-writer/
//
// scripts/verify-quiet-pty.py captures it in a real tmux PTY to prove the live
// row keeps changing through the quiet interval.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

const evidenceLines = 6

func main() {
	quiet := flag.Duration("quiet", 900*time.Millisecond, "how long the task stays Running without output")
	flag.Parse()

	evo.Init(evo.Config{Title: "quiet-writer"})
	os.Exit(evo.Main(func(ctx context.Context) error {
		build := evo.Task("build")
		build.Define(func(ctx context.Context) error {
			return buildQuietly(ctx, build, *quiet)
		})
		return nil
	}))
}

// buildQuietly writes the evidence lines, then waits without output. The
// wait is the real shape under test: a child process that has gone silent.
func buildQuietly(ctx context.Context, build *evo.TaskHandle, quiet time.Duration) error {
	log := build.Writer()
	for line := 1; line <= evidenceLines; line++ {
		if _, err := io.WriteString(log, "compile unit "+strconv.Itoa(line)+" of "+strconv.Itoa(evidenceLines)+"\n"); err != nil {
			return fmt.Errorf("write evidence line %d: %w", line, err)
		}
	}
	build.Doing("waiting")
	select {
	case <-time.After(quiet):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
