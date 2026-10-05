package engine

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"
)

// spanLog records when each Task's callback ran.
type spanLog struct {
	mu    sync.Mutex
	spans map[string][2]time.Time
	order []string
}

func (l *spanLog) work(name string, d time.Duration) func(context.Context) error {
	return func(context.Context) error {
		start := time.Now()
		l.mu.Lock()
		l.order = append(l.order, name)
		l.mu.Unlock()
		time.Sleep(d)
		l.mu.Lock()
		if l.spans == nil {
			l.spans = map[string][2]time.Time{}
		}
		l.spans[name] = [2]time.Time{start, time.Now()}
		l.mu.Unlock()
		return nil
	}
}

// overlapping names every pair of Tasks whose callbacks ran at once.
func (l *spanLog) overlapping() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []string
	for i, a := range l.order {
		for _, b := range l.order[i+1:] {
			sa, sb := l.spans[a], l.spans[b]
			if sa[0].Before(sb[1]) && sb[0].Before(sa[1]) {
				out = append(out, a+"||"+b)
			}
		}
	}
	return out
}

func (l *spanLog) started() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.order)
}

// TestSequenceLateStepMember_RunsInDeclarationOrder pins E-092: a Task
// declared into a Sequence step the Sequence has already moved past ran
// beside the later step or before it depending only on how long the
// step's earlier members took. Declared after step b, it now runs after
// b, and the step declared after it waits for it: one Running child, in
// declaration order, whatever the durations.
func TestSequenceLateStepMember_RunsInDeclarationOrder(t *testing.T) {
	for _, xDur := range []time.Duration{5 * time.Millisecond, 200 * time.Millisecond} {
		t.Run(fmt.Sprint(xDur), func(t *testing.T) {
			out := newGraphTestOutput(t)
			var log spanLog
			s := out.Sequence("s")
			g := s.Group("g")
			g.Task("x").Define(log.work("x", xDur))
			time.Sleep(20 * time.Millisecond)
			s.Task("b").Define(log.work("b", 100*time.Millisecond))
			time.Sleep(10 * time.Millisecond)
			g.Task("y").Define(log.work("y", 50*time.Millisecond))
			s.Task("c").Define(log.work("c", 10*time.Millisecond))
			if err := closeWithin(t, out); err != nil {
				t.Fatalf("Finish: %v", err)
			}
			if got, want := log.started(), []string{"x", "b", "y", "c"}; !slices.Equal(got, want) {
				t.Errorf("start order = %v, want %v", got, want)
			}
			if both := log.overlapping(); len(both) > 0 {
				t.Errorf("Sequence ran steps at once: %v", both)
			}
		})
	}
}

// TestAfterPopulatedGroup_LateMemberNeverGates pins E-093: a Group named
// in After while populated is taken as declared, so a child declared
// later never gates the dependent — whether or not the earlier children
// were still running when it was declared.
func TestAfterPopulatedGroup_LateMemberNeverGates(t *testing.T) {
	for _, aDur := range []time.Duration{0, 30 * time.Millisecond} {
		t.Run(fmt.Sprint(aDur), func(t *testing.T) {
			out := newGraphTestOutput(t)
			g := out.Group("g")
			g.Task("a").Define(func(context.Context) error { time.Sleep(aDur); return nil })
			fetched := make(chan struct{})
			out.Task("fetch").After(g).Define(func(context.Context) error { close(fetched); return nil })
			time.Sleep(10 * time.Millisecond)
			g.Task("b").Define(func(context.Context) error {
				select {
				case <-fetched:
					return nil
				case <-time.After(waitOutcomeTimeout):
					return fmt.Errorf("fetch waited for b, declared after After(g)")
				}
			})
			if err := closeWithin(t, out); err != nil {
				t.Fatalf("Finish: %v", err)
			}
		})
	}
}
