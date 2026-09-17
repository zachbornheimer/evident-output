package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/zachbornheimer/evident-output/internal/fingerprint"
	"github.com/zachbornheimer/evident-output/internal/manifest"
)

// execBasisRecords fingerprints spec.Basis for one Exec call (spec §11.1,
// §11.6): unlike basisRecordsFrom's plain File use, an FSPath entry here is
// first resolved against the workspace (spec §8.4: "FSPath Basis against
// workspace") and awaited on the freshness barrier for that canonical path
// — so a Basis input this Run's own Exec/File output claims is always
// fingerprinted only after its producing operation has settled (§11.6),
// never mid-write.
func (o *Output) execBasisRecords(ctx context.Context, basis []fingerprint.Fingerprint) ([]manifest.BasisRecord, error) {
	resolved := make([]fingerprint.Fingerprint, len(basis))
	for i, b := range basis {
		path, isPath := fingerprint.PathOf(b)
		if !isPath {
			resolved[i] = b
			continue
		}
		canon := o.resolveWorkspacePath(path)
		if err := o.awaitOutputBarrier(ctx, canon); err != nil {
			return nil, fmt.Errorf("evo: Exec Basis: %w", err)
		}
		resolved[i] = fingerprint.FSPath(canon)
	}
	return basisRecordsFrom(ctx, resolved)
}

// execDefinitionFingerprint computes Exec's operation definition fingerprint
// (spec §11.4/§8.4): resolved executable digest + argv + dir + sorted
// explicit Env + sorted Basis descriptors + sorted output paths. basis must
// already be canonicalized (basisRecordsFrom); outputs must already be
// sorted (see reconcileExec).
func execDefinitionFingerprint(executableDigest string, args []string, dir string, env map[string]string, basis []manifest.BasisRecord, sortedOutputs []string) string {
	h := sha256.New()
	_, _ = h.Write([]byte("evident-output:exec:definition:v1\x00"))
	_, _ = h.Write([]byte(executableDigest))
	h.Write([]byte{0})
	for _, a := range args {
		_, _ = h.Write([]byte(a))
		h.Write([]byte{0})
	}
	_, _ = h.Write([]byte(dir))
	h.Write([]byte{0})
	for _, k := range sortedEnvKeys(env) {
		_, _ = fmt.Fprintf(h, "%s=%s\x00", k, env[k])
	}
	for _, b := range basis {
		_, _ = h.Write([]byte(b.Kind))
		h.Write([]byte{0})
		_, _ = h.Write([]byte(b.Key))
		h.Write([]byte{0})
		_, _ = h.Write([]byte(b.Digest))
		h.Write([]byte{0})
	}
	for _, out := range sortedOutputs {
		_, _ = h.Write([]byte(out))
		h.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// sortedEnvKeys returns env's keys sorted, so two ExecSpecs with the same
// explicit Env entries always hash identically regardless of map iteration
// order (spec §11.4: "sorted explicit Env").
func sortedEnvKeys(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// execOperationCurrent reports whether prior still matches this call's
// definition, Basis, and every declared output's current on-disk digest
// (spec §11.4/§11.5). An ExecSpec with no declared Outputs is never current
// — spec §8.4: "No Outputs → always run", since Evo has nothing observable
// to prove the prior run's effect still holds.
func execOperationCurrent(ctx context.Context, prior manifest.OperationRecord, hasPrior bool, defFingerprint string, basis []manifest.BasisRecord, sortedOutputs []string) (bool, error) {
	if len(sortedOutputs) == 0 {
		return false, nil
	}
	if !hasPrior || prior.DefinitionFingerprint != defFingerprint || len(prior.Outputs) != len(sortedOutputs) {
		return false, nil
	}
	if !basisRecordsEqual(prior.Basis, basis) {
		return false, nil
	}
	for i, out := range sortedOutputs {
		if prior.Outputs[i].Path != out {
			return false, nil
		}
		digest, err := pathOutputDigest(ctx, out)
		if err != nil {
			return false, err
		}
		if prior.Outputs[i].Digest != digest {
			return false, nil
		}
	}
	return true, nil
}

// execConsultManifest resolves this Exec call's prior operation record (if
// any) and reports whether it is still current (spec §11.4/§11.5/§8.4),
// mirroring fileConsultManifest's shape. prior/defFingerprint/basis are
// always returned so the caller can forward a current hit unchanged, or
// carry the fresh definition into the post-spawn success record.
func (o *Output) execConsultManifest(ctx context.Context, taskID string, spec ExecSpec, target execTarget) (current bool, prior manifest.OperationRecord, defFingerprint string, basis []manifest.BasisRecord, err error) {
	store, openErr := o.manifestFor(ctx)
	if openErr != nil {
		return false, manifest.OperationRecord{}, "", nil, fmt.Errorf("evo: Exec %q: %w", spec.Executable, openErr)
	}
	o.emitManifestWarningOnce(store.Warning())

	basis, err = o.execBasisRecords(ctx, spec.Basis)
	if err != nil {
		return false, manifest.OperationRecord{}, "", nil, err
	}

	executableDigest, digestErr := pathOutputDigest(ctx, target.ExecutablePath)
	if digestErr != nil {
		return false, manifest.OperationRecord{}, "", nil, fmt.Errorf("evo: Exec %q: %w", spec.Executable, digestErr)
	}
	defFingerprint = execDefinitionFingerprint(executableDigest, spec.Args, target.Dir, spec.Env, basis, target.Outputs)

	o.mu.Lock()
	key, ord, ok := o.taskManifestKeyLocked(taskID)
	o.mu.Unlock()
	if !ok {
		return false, manifest.OperationRecord{}, "", nil, ErrNoTaskContext
	}

	priorRecord, hasPrior := store.Operation(key, ord)
	isCurrent, checkErr := execOperationCurrent(ctx, priorRecord, hasPrior, defFingerprint, basis, target.Outputs)
	if checkErr != nil {
		return false, manifest.OperationRecord{}, "", nil, fmt.Errorf("evo: Exec %q: %w", spec.Executable, checkErr)
	}
	return isCurrent, priorRecord, defFingerprint, basis, nil
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
	o.mu.Lock()
	claimed := make([]string, 0, len(outputs))
	for _, out := range outputs {
		if claimErr := o.claimManifestOutputLocked(taskID, out); claimErr != nil {
			for _, done := range claimed {
				o.settleOutputBarrierLocked(done)
			}
			o.mu.Unlock()
			return nil, claimErr
		}
		claimed = append(claimed, out)
	}
	o.mu.Unlock()
	return func() {
		for _, out := range claimed {
			o.settleOutputBarrier(out)
		}
	}, nil
}
