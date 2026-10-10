package engine

import (
	"context"
	"fmt"
	"sort"

	"github.com/zachbornheimer/evident-output/internal/freshness"
	"github.com/zachbornheimer/evident-output/internal/wire"
)

// execBasisRecords fingerprints spec.Basis for one Exec call (spec §11.1,
// §11.6): unlike a plain File use, an FSPath entry here is first resolved
// against the workspace (spec §8.4: "FSPath Basis against workspace") and
// awaited on the freshness barrier for that canonical path — so a Basis
// input this Run's own Exec/File output claims is always fingerprinted only
// after its producing operation has settled (§11.6), never mid-write. Each
// FSPath entry is then observed under a read claim (observeBasis), which
// also excludes File commits from other Outputs.
func (o *Output) execBasisRecords(ctx context.Context, basis []freshness.Fingerprint) ([]freshness.BasisRecord, error) {
	resolved := make([]freshness.Fingerprint, len(basis))
	for i, b := range basis {
		path, isPath := freshness.PathOf(b)
		if !isPath {
			resolved[i] = b
			continue
		}
		canon := o.resolveWorkspacePath(path)
		if err := o.outputBarrier().Await(ctx, canon); err != nil {
			return nil, fmt.Errorf("evo: Exec Basis: %w", err)
		}
		resolved[i] = freshness.FSPath(canon)
	}
	return o.observeBasis(ctx, resolved)
}

// execConsultManifest resolves this Exec call's freshness against the
// manifest (spec §11.4/§11.5/§8.4): whether it is current, planned only, or
// must spawn, with the fresh definition and Basis the caller commits after a
// successful run.
func (o *Output) execConsultManifest(ctx context.Context, taskID string, spec ExecSpec, target execTarget) (freshness.ExecEvaluation, error) {
	store, openErr := o.manifestFor(ctx)
	if openErr != nil {
		return freshness.ExecEvaluation{}, fmt.Errorf("evo: Exec %q: %w", spec.Executable, openErr)
	}
	o.emitManifestWarningOnce(store.Warning())

	basis, err := o.execBasisRecords(ctx, spec.Basis)
	if err != nil {
		return freshness.ExecEvaluation{}, err
	}
	o.mu.Lock()
	o.emitWireEventLocked(wire.EventBasisFingerprinted, taskID, map[string]any{
		"kind": "exec", "executable": spec.Executable, "count": len(basis),
	})
	key, ord, ok := o.taskManifestKeyLocked(taskID)
	o.mu.Unlock()
	if !ok {
		return freshness.ExecEvaluation{}, ErrNoTaskContext
	}

	call := freshness.ExecCall{
		ExecutablePath: target.ExecutablePath,
		Args:           spec.Args,
		Dir:            target.Dir,
		Env:            spec.Env,
		Basis:          basis,
		Outputs:        target.Outputs,
	}
	evaluation, evaluateErr := call.Evaluate(ctx, store, key, ord, o.DryRun())
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

// claimExecOutputs claims every one of outputs for taskID (spec §8.3/§11.4:
// two operations claiming one canonical output conflict), unwinding any
// already-claimed-by-this-call outputs before returning the conflict so a
// partially claimed ExecSpec never leaves other outputs' barriers dangling
// open. The returned release func settles every claimed output's barrier
// exactly once and must be deferred by the caller.
func (o *Output) claimExecOutputs(taskID string, outputs []string) (release func(), err error) {
	barrier := o.outputBarrier()
	o.mu.Lock()
	claimed := make([]string, 0, len(outputs))
	for _, out := range outputs {
		if claimErr := o.claimManifestOutputLocked(taskID, out); claimErr != nil {
			for _, done := range claimed {
				barrier.Settle(done)
			}
			o.mu.Unlock()
			return nil, claimErr
		}
		claimed = append(claimed, out)
	}
	o.mu.Unlock()
	return func() {
		for _, out := range claimed {
			barrier.Settle(out)
		}
	}, nil
}
