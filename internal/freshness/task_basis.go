// This file judges a Task's declared Basis as the Task starts: what it observes
// and whether the last successful Run recorded the same identity.

package freshness

import (
	"context"
	"fmt"
	"slices"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// BasisSource is one Task Basis input: something whose content identity is
// observed when the Task starts. Build one with FingerprintBasis.
type BasisSource interface {
	observe(ctx context.Context) (BasisRecord, error)
}

// FingerprintBasis observes a Fingerprint (FSPath, Value, App).
func FingerprintBasis(f Fingerprint) BasisSource { return fingerprintBasis{inner: f} }

type fingerprintBasis struct{ inner Fingerprint }

func (b fingerprintBasis) observe(ctx context.Context) (BasisRecord, error) {
	records, err := basisRecordsFrom(ctx, []Fingerprint{b.inner})
	if err != nil {
		return BasisRecord{}, err
	}
	return records[0], nil
}

// StoreOpener returns the Run's manifest Store, opening it on first use. The
// caller supplies it so the errors it reports keep the caller's identity;
// ManifestSession.Store is the plain one.
type StoreOpener func(ctx context.Context) (*ManifestStore, error)

// BasisJudgement is the verdict of judging a Task's declared Basis.
type BasisJudgement struct {
	// Declared is false when the Task declared no Basis; nothing else is set.
	Declared bool
	// Observed is the Basis as observed now, canonicalized.
	Observed []BasisRecord
	// Current is true when the Task's last successful Run recorded the same
	// identity and its recorded outputs still hold.
	Current bool
}

// DeclaresBasis reports whether id declared a Basis to judge.
func (t *TaskTable) DeclaresBasis(id record.TaskID) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	entry := t.tasks[id]
	return entry != nil && len(entry.inputs) > 0
}

// JudgeBasis observes the Task's Basis as the Task starts and reports whether the
// Task's last successful Run recorded the same identity. The observation
// holds no resource claim, so it never orders or blocks any other Task. A
// Task with no Basis is never current. Without a prior record the Task runs.
// A current Task adopts its prior operations as its own pending ones, so a
// later commit rewrites identical state. A planned Run judges the same way but
// remembers nothing, so its observed Basis is never committed, and a Basis
// path an operation it planned would change is never current.
func (t *TaskTable) JudgeBasis(ctx context.Context, openStore StoreOpener, task TaskView, mode RunMode) (BasisJudgement, error) {
	id := task.TaskID()
	t.mu.Lock()
	var inputs []BasisSource
	if entry := t.tasks[id]; entry != nil {
		inputs = slices.Clone(entry.inputs)
	}
	t.mu.Unlock()
	if len(inputs) == 0 {
		return BasisJudgement{}, nil
	}
	observed, err := observeTaskBasis(ctx, inputs)
	if err != nil {
		return BasisJudgement{}, err
	}
	store, err := openStore(ctx)
	if err != nil {
		return BasisJudgement{}, fmt.Errorf("evo: Basis: %w", err)
	}
	prior, ok := store.Task(task.ManifestKey())
	operations, priorBasis := splitBasisOperation(prior.Operations)
	current := ok && priorBasis != nil && BasisRecordsEqual(priorBasis, observed)
	if mode == RunPlans && current {
		current = !t.plansChange(observed)
	}
	if current {
		if current, err = RecordedOutputsHold(ctx, operations); err != nil {
			return BasisJudgement{}, err
		}
	}
	judgement := BasisJudgement{Declared: true, Observed: observed, Current: current}
	if !mode.Remembers() {
		return judgement, nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	entry := t.entryLocked(id)
	entry.observed = observed
	if current {
		entry.operations = operations
	}
	return judgement, nil
}

// observeTaskBasis observes every input and canonicalizes the result. Basis
// order is not identity; a repeated (kind, key) is a programmer error.
func observeTaskBasis(ctx context.Context, inputs []BasisSource) ([]BasisRecord, error) {
	records := make([]BasisRecord, 0, len(inputs))
	for _, in := range inputs {
		rec, err := in.observe(ctx)
		if err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	return canonicalBasis(records)
}

// splitBasisOperation separates a committed task's trailing Basis record
// from its other operations. basis is nil when the task recorded none.
func splitBasisOperation(ops []OperationRecord) (rest []OperationRecord, basis []BasisRecord) {
	if n := len(ops); n > 0 && ops[n-1].Kind == TaskBasisOperationKind {
		return slices.Clone(ops[:n-1]), ops[n-1].Basis
	}
	return slices.Clone(ops), nil
}
