package testkit

import (
	"io"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// New builds an Isolated, plain, non-interactive *Output pre-wired for a
// test: it never touches package-level default state (t.Parallel-safe),
// never opens a live region (no spinner goroutine to leak), and is closed
// automatically via t.Cleanup — replacing the
// evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard)}}) plus a
// hand-written t.Cleanup(func() { _ = out.Close() }) boilerplate repeated at
// every test call site that only needs a working *Output to assert against.
func New(t *testing.T) *evo.Output {
	t.Helper()
	out := evo.Init(evo.Config{
		Isolated: true,
		Plain:    true,
		Stdout:   io.Discard,
		Stderr:   io.Discard,
	})
	t.Cleanup(func() { _ = out.Close() })
	return out
}
