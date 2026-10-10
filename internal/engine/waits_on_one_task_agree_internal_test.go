package engine

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// besideStallCeilings are the concurrency ceilings the stall below must resolve the
// same way at.
var besideStallCeilings = []int{1, 2, 4, 8}

// stallBesideASettledOwingTask runs a genuine wait cycle (a and b wait on each
// other) next to c, which settles itself Blocked and then waits on a, so its
// own error is still owed. m waits on c while that is so. It returns what m
// got, then what a, b and c answer once the run finished.
func stallBesideASettledOwingTask(t *testing.T, ceiling int) (m, a, b, c error) {
	t.Helper()
	out := quietOutput(Config{MaxConcurrency: ceiling})
	t.Cleanup(func() { _ = out.Close() })
	ta, tb, tc := out.Task("a"), out.Task("b"), out.Task("c")
	ta.Define(func(context.Context) error { time.Sleep(5 * time.Millisecond); return tb.Wait() })
	tb.Define(func(context.Context) error { time.Sleep(5 * time.Millisecond); return ta.Wait() })
	tc.Define(func(context.Context) error { tc.Block("held"); time.Sleep(5 * time.Millisecond); return ta.Wait() })
	var mSaw error
	var seen sync.Mutex
	out.Task("m").Define(func(context.Context) error {
		time.Sleep(10 * time.Millisecond)
		seen.Lock()
		defer seen.Unlock()
		mSaw = tc.Wait()
		return nil
	})
	if _, finished := within(5*time.Second, out.Finish); !finished {
		t.Fatal("Finish hung")
	}
	seen.Lock()
	defer seen.Unlock()
	return mSaw, ta.Wait(), tb.Wait(), tc.Wait()
}

// Wait returns the error its callback returned, so two Waits on one Task agree
// whether one of them was parked while the run stalled and the other came
// after, at every ceiling.
func TestSlice36_TwoWaitsOnOneTaskReturnTheErrorItsCallbackReturned(t *testing.T) {
	for _, ceiling := range besideStallCeilings {
		t.Run(fmt.Sprint(ceiling), func(t *testing.T) {
			parked, _, _, later := stallBesideASettledOwingTask(t, ceiling)
			if fmt.Sprint(parked) != fmt.Sprint(later) {
				t.Errorf("c.Wait() parked during the stall = %v; c.Wait() after = %v", parked, later)
			}
		})
	}
}

// The wait cycle beside the settled Task is still named, whatever the ceiling.
func TestSlice36_ACycleBesideASettledOwingTaskIsStillNamed(t *testing.T) {
	for _, ceiling := range besideStallCeilings {
		t.Run(fmt.Sprint(ceiling), func(t *testing.T) {
			_, a, b, _ := stallBesideASettledOwingTask(t, ceiling)
			if !errors.Is(a, ErrWaitDeadlock) || !errors.Is(b, ErrWaitDeadlock) {
				t.Errorf("cycle not named: a=%v b=%v", a, b)
			}
		})
	}
}
