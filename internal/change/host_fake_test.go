package change

import (
	"context"
	"sync"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/graph"
	"github.com/zachbornheimer/evident-output/internal/record"
)

// fakeHost is a Host that keeps what a Run's changes told it.
type fakeHost struct {
	mu        sync.Mutex
	applies   bool
	ledgerErr error
	entries   []record.LedgerEntry
	waiting   []string
	restored  int
}

func (h *fakeHost) Ledger(record.TaskID) (Ledger, error) {
	if h.ledgerErr != nil {
		return nil, h.ledgerErr
	}
	return fakeLedger{host: h}, nil
}

func (h *fakeHost) ShowWaiting(_ record.TaskID, label string) func() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.waiting = append(h.waiting, label)
	return func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.restored++
	}
}

func (h *fakeHost) recorded() []record.LedgerEntry {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]record.LedgerEntry(nil), h.entries...)
}

type fakeLedger struct{ host *fakeHost }

func (l fakeLedger) Applies() bool { return l.host.applies }

func (l fakeLedger) Record(e record.LedgerEntry) {
	l.host.mu.Lock()
	defer l.host.mu.Unlock()
	l.host.entries = append(l.host.entries, e)
}

// scopedRun is a Run over a fakeHost and the context of a Define callback
// running Task t1 under it.
func scopedRun(t *testing.T, host *fakeHost) (*Run, context.Context) {
	t.Helper()
	g := graph.New(record.NewRun())
	run := NewRun(host, g)
	return run, graph.WithScope(context.Background(), graph.NewScope(g, "t1", run))
}
