package change

import (
	"context"
	"sync"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/freshness"
	sysfs "github.com/zachbornheimer/evident-output/internal/fs"
	"github.com/zachbornheimer/evident-output/internal/graph"
	"github.com/zachbornheimer/evident-output/internal/record"
)

// fakeHost is a Host that keeps what a Run's changes told it. Its filesystem
// is the real disk, so tests work in temporary directories; its manifest is
// a real session in a temporary state directory that remembers nothing a
// Task has not committed.
type fakeHost struct {
	mu        sync.Mutex
	applies   bool
	ledgerErr error
	entries   []record.LedgerEntry
	waiting   []string
	restored  int

	observed []Observation
	attached []record.VerificationDetail
	appended []freshness.OperationRecord
	planned  int
	spawned  []SpawnCommand
	spawn    func(SpawnCommand) (ExecResult, error)

	fsys     sysfs.FS
	manifest *freshness.ManifestSession
}

// newFakeHost is an applying host over the real disk, with a manifest kept in
// stateDir.
func newFakeHost(stateDir string) *fakeHost {
	return &fakeHost{
		applies: true,
		fsys:    sysfs.Disk{},
		manifest: freshness.NewManifestSession(func() freshness.ManifestConfig {
			return freshness.ManifestConfig{AppID: "change-test", StateDir: stateDir, Workspace: stateDir}
		}),
	}
}

func (h *fakeHost) DryRun() bool { return !h.applies }

func (h *fakeHost) FS() sysfs.FS {
	if h.fsys == nil {
		return sysfs.Disk{}
	}
	return h.fsys
}

func (h *fakeHost) Ledger(record.TaskID) (Ledger, error) {
	if h.ledgerErr != nil {
		return nil, h.ledgerErr
	}
	return fakeLedger{host: h}, nil
}

func (h *fakeHost) AttachVerification(_ record.TaskID, details []record.VerificationDetail) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.attached = append(h.attached, details...)
}

func (h *fakeHost) Observe(ob Observation) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.observed = append(h.observed, ob)
}

func (h *fakeHost) Manifest(ctx context.Context) (*freshness.ManifestStore, error) {
	return h.manifest.Store(ctx)
}

func (h *fakeHost) NoteManifestWarning(error) {}

func (h *fakeHost) TaskManifest(record.TaskID) (string, int, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return "task:/t1", len(h.appended) + h.planned, true
}

func (h *fakeHost) AppendOperation(_ record.TaskID, rec freshness.OperationRecord) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.appended = append(h.appended, rec)
}

func (h *fakeHost) PlanOperation(record.TaskID) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.planned++
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

func (h *fakeHost) Spawn(_ context.Context, _ record.TaskID, cmd SpawnCommand) (ExecResult, error) {
	h.mu.Lock()
	h.spawned = append(h.spawned, cmd)
	spawn := h.spawn
	h.mu.Unlock()
	if spawn == nil {
		return ExecResult{Ran: true}, nil
	}
	return spawn(cmd)
}

func (h *fakeHost) recorded() []record.LedgerEntry {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]record.LedgerEntry(nil), h.entries...)
}

// observations is the kinds of what the changes narrated, in order.
func (h *fakeHost) observations() []ObservationKind {
	h.mu.Lock()
	defer h.mu.Unlock()
	kinds := make([]ObservationKind, len(h.observed))
	for i, ob := range h.observed {
		kinds[i] = ob.Kind
	}
	return kinds
}

type fakeLedger struct{ host *fakeHost }

func (l fakeLedger) Applies() bool { return l.host.applies }

func (l fakeLedger) Record(e record.LedgerEntry) {
	l.host.mu.Lock()
	defer l.host.mu.Unlock()
	l.host.entries = append(l.host.entries, e)
}

// scopedRun is a Run over host and the context of a Define callback running
// Task t1 under it.
func scopedRun(t *testing.T, host *fakeHost) (*Run, context.Context) {
	t.Helper()
	g := graph.New(record.NewRun())
	run := NewRun(host, g)
	return run, graph.WithScope(context.Background(), graph.NewScope(g, "t1", run))
}
