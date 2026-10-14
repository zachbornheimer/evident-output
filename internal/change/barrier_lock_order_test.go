package change

import (
	"errors"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/freshness"
)

// lockCheckingGates is the graph's gate table as a Run sees it, failing the
// test when a gate is opened or settled under the Run's outputsMu: a gate
// call takes graph.mu, and its unlock can deliver record listeners that take
// the run's lock, which a change leaf must never be held across.
type lockCheckingGates struct {
	freshness.BarrierTable
	t   *testing.T
	run *Run
}

func (g lockCheckingGates) check(call, path string) {
	if !g.run.outputsMu.TryLock() {
		g.t.Errorf("%s(%s) ran with outputsMu held", call, path)
		return
	}
	g.run.outputsMu.Unlock()
}

func (g lockCheckingGates) OpenBarrier(path string) {
	g.check("OpenBarrier", path)
	g.BarrierTable.OpenBarrier(path)
}

func (g lockCheckingGates) SettleBarrier(path string) {
	g.check("SettleBarrier", path)
	g.BarrierTable.SettleBarrier(path)
}

func TestBarrierCallsRunOutsideOutputsLock(t *testing.T) {
	run, _ := scopedRun(t, newFakeHost(t.TempDir()))
	run.barrier = freshness.NewOutputBarrier(lockCheckingGates{BarrierTable: run.graph, t: t, run: run})

	if err := run.claimOutput("file", "/out/file"); err != nil {
		t.Fatalf("File claim = %v", err)
	}
	release, err := run.claimExecOutputs("exec", []string{"/out/a", "/out/b"})
	if err != nil {
		t.Fatalf("Exec claim = %v", err)
	}
	if _, ok := run.graph.Barrier("/out/a"); !ok {
		t.Error("an Exec claim opened no gate for its output")
	}
	release()

	// A clash midway leaves no gate of the losing claim open.
	if _, err := run.claimExecOutputs("loser", []string{"/out/c", "/out/file"}); !errors.Is(err, ErrFileConflictingProducer) {
		t.Fatalf("clashing Exec claim = %v, want ErrFileConflictingProducer", err)
	}
	if settled, ok := run.graph.Barrier("/out/c"); ok {
		select {
		case <-settled:
		default:
			t.Error("the losing claim left /out/c's gate open")
		}
	}
}
