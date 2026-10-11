// This file owns what Exec asks of freshness: claiming its declared outputs,
// fingerprinting its Basis after the producers of those inputs settled, and
// consulting the manifest.

package change

import (
	"context"
	"fmt"
	"sort"

	"github.com/zachbornheimer/evident-output/internal/freshness"
	"github.com/zachbornheimer/evident-output/internal/graph"
	"github.com/zachbornheimer/evident-output/internal/record"
)

// execBasisRecords fingerprints spec.Basis for one Exec call (spec §11.1,
// §11.6): unlike a plain File use, an FSPath entry here is first resolved
// against the workspace (spec §8.4: "FSPath Basis against workspace") and
// awaited on the freshness barrier for that canonical path — so a Basis
// input this Run's own Exec/File output claims is always fingerprinted only
// after its producing operation has settled (§11.6), never mid-write. Each
// FSPath entry is then observed under a read claim (ObserveBasis), which
// also excludes File commits from other Runs.
func (r *Run) execBasisRecords(ctx context.Context, basis []freshness.Fingerprint) ([]freshness.BasisRecord, error) {
	resolved := make([]freshness.Fingerprint, len(basis))
	for i, b := range basis {
		path, isPath := freshness.PathOf(b)
		if !isPath {
			resolved[i] = b
			continue
		}
		canon := r.resolvePath(path)
		if awaitErr := r.barrier.Await(ctx, canon); awaitErr != nil {
			return nil, fmt.Errorf("evo: Exec Basis: %w", awaitErr)
		}
		resolved[i] = freshness.FSPath(canon)
	}
	return r.ObserveBasis(ctx, resolved)
}

// consultExec resolves this Exec call's freshness against the manifest (spec
// §11.4/§11.5/§8.4): whether it is current, planned only, or must spawn,
// with the fresh definition and Basis the caller commits after a successful
// run.
func (r *Run) consultExec(ctx context.Context, id record.TaskID, spec ExecSpec, target execTarget) (freshness.ExecEvaluation, error) {
	store, openErr := r.host.Manifest(ctx)
	if openErr != nil {
		return freshness.ExecEvaluation{}, fmt.Errorf("evo: Exec %q: %w", spec.Executable, openErr)
	}
	r.host.NoteManifestWarning(store.Warning())

	basis, basisErr := r.execBasisRecords(ctx, spec.Basis)
	if basisErr != nil {
		return freshness.ExecEvaluation{}, basisErr
	}
	r.host.Observe(Observation{Task: id, Kind: BasisFingerprinted, Subject: Subject{Kind: SubjectExec, Name: spec.Executable}, Count: len(basis)})
	key, ordinal, ok := r.host.TaskManifest(id)
	if !ok {
		return freshness.ExecEvaluation{}, graph.ErrNoTaskContext
	}

	call := freshness.ExecCall{
		ExecutablePath: target.ExecutablePath,
		Args:           spec.Args,
		Dir:            target.Dir,
		Env:            spec.Env,
		Basis:          basis,
		Outputs:        target.Outputs,
	}
	evaluation, evaluateErr := call.Evaluate(ctx, store, key, ordinal, r.host.DryRun())
	if evaluateErr != nil {
		return freshness.ExecEvaluation{}, fmt.Errorf("evo: Exec %q: %w", spec.Executable, evaluateErr)
	}
	return evaluation, nil
}

// resolveExecOutputs resolves every declared Output against dir (spec
// §8.4: "relative Outputs against Dir") and returns them sorted so both
// the definition fingerprint and the manifest's Outputs order are
// deterministic regardless of declaration order.
func resolveExecOutputs(dir string, outputs []string) []string {
	resolved := make([]string, len(outputs))
	for i, out := range outputs {
		resolved[i] = resolvePathAgainst(dir, out)
	}
	sort.Strings(resolved)
	return resolved
}

// claimExecOutputs claims every one of outputs for id (spec §8.3/§11.4: two
// operations claiming one canonical output conflict), unwinding any
// already-claimed-by-this-call outputs before returning the conflict so a
// partially claimed ExecSpec never leaves other outputs' barriers dangling
// open. The returned release func settles every claimed output's barrier
// exactly once and must be deferred by the caller.
func (r *Run) claimExecOutputs(id record.TaskID, outputs []string) (release func(), err error) {
	r.outputsMu.Lock()
	claimed := make([]string, 0, len(outputs))
	for _, out := range outputs {
		if claimErr := r.claimOutputLocked(id, out); claimErr != nil {
			for _, done := range claimed {
				r.barrier.Settle(done)
			}
			r.outputsMu.Unlock()
			return nil, claimErr
		}
		claimed = append(claimed, out)
	}
	r.outputsMu.Unlock()
	return func() {
		for _, out := range claimed {
			r.barrier.Settle(out)
		}
	}, nil
}
