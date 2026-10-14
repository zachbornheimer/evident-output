package evo

import (
	"context"
	"io"
	"testing"
)

type nestedOutcome struct {
	outer, n1, n2 error
	read          int
}

// runFinishedNested: outer (After make) lends its slot to n1 (After make) at
// the given ceiling. n1's callback starts goroutine Y and returns while Y
// lives. outer then lends its slot to n2, which is NOT ordered after make.
// While n2 runs, Y reads make's Computed. Y is n1's hands (n1 is After make)
// and runs on outer's goroutine lineage (outer is After make): neither n2
// nor anyone else did anything unordered. n2 never reads anything.
func runFinishedNested(t *testing.T, ceiling int) nestedOutcome {
	t.Helper()
	out := initRun(Config{stdout: io.Discard, plain: true, MaxConcurrency: ceiling})
	defer func() { _ = out.close() }()
	made := out.Group("producers").Task("make")
	producer := Compute(made, func(context.Context) (int, error) { return 7, nil })
	n1 := out.Group("nested").Task("n1").After(made)
	n2 := out.Group("other").Task("n2")
	n2Running, readDone := make(chan struct{}), make(chan struct{})
	var read int
	outer := out.Group("consumers").Task("outer").After(made)
	outer.Define(func(context.Context) error {
		n1.Define(func(context.Context) error {
			go func() {
				<-n2Running
				read = producer.Get()
				close(readDone)
			}()
			return nil
		})
		if err := n1.wait(); err != nil {
			return err
		}
		n2.Define(func(context.Context) error {
			close(n2Running)
			<-readDone
			return nil
		})
		return n2.wait()
	})
	_ = out.finish()
	return nestedOutcome{outer: outer.wait(), n1: n1.wait(), n2: n2.wait(), read: read}
}

func TestGoroutineOfAFinishedNestedTaskIsNotTheNextNestedTasksAtCeiling2(t *testing.T) {
	got := runFinishedNested(t, 2)
	if got.outer != nil || got.n1 != nil || got.n2 != nil || got.read != 7 {
		t.Errorf("outer=%v n1=%v n2=%v read=%d; want nil nil nil 7", got.outer, got.n1, got.n2, got.read)
	}
}
