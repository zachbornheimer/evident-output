// This file owns Run, the freshness side of one Run: the manifest it reads
// and commits to, and what it remembers about each Task the graph declared.

package freshness

import (
	"context"
	"errors"
	"maps"
	"slices"

	"github.com/zachbornheimer/evident-output/internal/graph"
	"github.com/zachbornheimer/evident-output/internal/record"
)

// RunConfig configures a Run.
type RunConfig struct {
	// Where names the manifest the Run reads, asked on first use.
	Where func() ManifestConfig
	// Closed is the error the Run reports for its manifest once it is
	// closed: the caller's own sentinel, so a call after Close fails the
	// way every other call after Close does.
	Closed error
	// DryRun reports whether the run only plans its work.
	DryRun func() bool
	// Narration hears what the Run judged about each Task.
	Narration Narration
}

// Run decides, for one Run, whether each Task's work is needed. It finds a
// Task by its id in the graph, so the caller keeps no freshness state of its
// own. It is safe for concurrent use and takes no lock of the caller's.
type Run struct {
	graph     *graph.Graph
	session   *ManifestSession
	tasks     *TaskTable
	closed    error
	dryRun    func() bool
	narration Narration
}

// NewRun is the freshness side of the Run whose Tasks g declares.
func NewRun(g *graph.Graph, cfg RunConfig) *Run {
	r := &Run{
		graph: g, session: NewManifestSession(cfg.Where), tasks: NewTaskTable(),
		closed: cfg.Closed, dryRun: cfg.DryRun, narration: cfg.Narration,
	}
	if r.closed == nil {
		r.closed = ErrManifestSessionClosed
	}
	if r.dryRun == nil {
		r.dryRun = func() bool { return false }
	}
	if r.narration == nil {
		r.narration = silence{}
	}
	return r
}

// ModeFor is the mode of a Run that only plans its work when dryRun is set.
func ModeFor(dryRun bool) RunMode {
	if dryRun {
		return RunPlans
	}
	return RunApplies
}

// Session is the Run's manifest session.
func (r *Run) Session() *ManifestSession { return r.session }

// Tasks is what the Run remembers about each Task.
func (r *Run) Tasks() *TaskTable { return r.tasks }

// Store returns the Run's manifest Store, opening it on first use (spec
// §11.3). A miss or an open failure degrades the Run to live-filesystem-only
// behavior; see ManifestSession.Store.
func (r *Run) Store(ctx context.Context) (*ManifestStore, error) {
	store, err := r.session.Store(ctx)
	if errors.Is(err, ErrManifestSessionClosed) {
		return nil, r.closed
	}
	return store, err
}

// task is the declared Task named id, or nil.
func (r *Run) task(id record.TaskID) *graph.Task { return r.graph.Task(string(id)) }

// TaskManifest is id's stable manifest key (§3.1) and the ordinal of its
// next operation. ok is false when the Task is unknown.
func (r *Run) TaskManifest(id record.TaskID) (key string, ordinal int, ok bool) {
	t := r.task(id)
	if t == nil {
		return "", 0, false
	}
	return t.ManifestKey(), r.tasks.OperationCount(id), true
}

// AppendOperation records rec as id's next operation, committed only if the
// Task settles Done (spec §8.2/§11.3).
func (r *Run) AppendOperation(id record.TaskID, rec OperationRecord) {
	if r.task(id) != nil {
		r.tasks.AppendOperation(id, rec)
	}
}

// PlanOperation holds id's next operation's place without a record: it was
// found stale and, in a planned Run, left unapplied. changes are the paths
// it would have changed.
func (r *Run) PlanOperation(id record.TaskID, changes ...string) {
	if r.task(id) != nil {
		r.tasks.PlanOperation(id, changes...)
	}
}

// Plan is how this Run judges an operation's freshness: the zero Plan when
// it applies its work.
func (r *Run) Plan() Plan {
	if r.dryRun() {
		return r.tasks.Plan()
	}
	return Plan{}
}

// Commit persists t's accumulated operations once it settled Done; see
// TaskTable.Commit. The caller already holds t, so the commit never looks it
// up under the graph's lock.
func (r *Run) Commit(ctx context.Context, t *graph.Task, mode RunMode) TaskCommit {
	return r.tasks.Commit(ctx, r.session, t, mode)
}

// Retract takes back t's committed record; see TaskTable.Retract.
func (r *Run) Retract(t *graph.Task, mode RunMode) {
	r.tasks.Retract(r.session, t, mode)
}

// JudgeBasis judges id's declared Basis as the Task starts; see
// TaskTable.JudgeBasis. An unknown Task declared none.
func (r *Run) JudgeBasis(ctx context.Context, id record.TaskID, mode RunMode) (BasisJudgement, error) {
	if !r.tasks.DeclaresBasis(id) {
		// Most Tasks declare none: asking first keeps them off the graph's
		// lock, which every scheduling step takes.
		return BasisJudgement{}, nil
	}
	t := r.task(id)
	if t == nil {
		return BasisJudgement{}, nil
	}
	return r.tasks.JudgeBasis(ctx, r.Store, t, mode)
}

// SpecBasis is the Basis a File or Exec spec declares: its explicit
// fingerprints, then one content fingerprint per input path and one per
// value, in key order, so a changed input byte or value makes the next run
// re-execute.
func SpecBasis(inputs []string, values map[string]string, explicit []Fingerprint) []Fingerprint {
	basis := slices.Clone(explicit)
	for _, path := range inputs {
		basis = append(basis, FSPath(path))
	}
	for _, name := range slices.Sorted(maps.Keys(values)) {
		basis = append(basis, Value(name, values[name]))
	}
	return basis
}

// silence is the Narration of a Run nobody reads.
type silence struct{}

func (silence) BasisJudged(record.TaskID, BasisJudgement)            {}
func (silence) VerifierObserved(record.TaskID, int, VerifierOutcome) {}
