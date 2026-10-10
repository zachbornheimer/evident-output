package evo_test

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// An unordered Get is a misuse whether or not the producer settled first:
// the order is declared, not observed, so a read that raced ahead of the
// producer must fail with ErrComputedUnordered and never yield a zero the
// callback can carry on with, in every mode.
func TestCompute_UnorderedGetBeforeTheProducerSettlesFailsInEveryMode(t *testing.T) {
	const attempts = 20
	modes := map[string]evo.Config{
		"default": {},
		"strict":  {Strict: true},
		"dry run": {DryRun: true},
	}
	for name, mode := range modes {
		t.Run(name, func(t *testing.T) {
			mode.Isolated, mode.Stdout, mode.Plain = true, io.Discard, true
			for range attempts {
				assertUnorderedGetBeforeSettleUnwinds(t, mode)
			}
		})
	}
}

func assertUnorderedGetBeforeSettleUnwinds(t *testing.T, mode evo.Config) {
	t.Helper()
	out := evo.Init(mode)
	defer func() { _ = out.Close() }()
	release := make(chan struct{})
	producer := evo.Compute(out.Group("producers").Task("make"), func(context.Context) (int, error) {
		<-release
		return 1, nil
	})
	var continued atomic.Bool
	read := out.Group("consumers").Task("read")
	read.Define(func(context.Context) error {
		defer close(release)
		_ = producer.Get()
		continued.Store(true)
		return nil
	})
	_ = out.Finish()
	if continued.Load() || !errors.Is(out.Err(), evo.ErrComputedUnordered) {
		t.Fatalf("continued=%v, Err() = %v; want the callback unwound with ErrComputedUnordered", continued.Load(), out.Err())
	}
}
