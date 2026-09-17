package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"

	"github.com/zachbornheimer/evident-output/internal/fingerprint"
	"github.com/zachbornheimer/evident-output/internal/manifest"
	"github.com/zachbornheimer/evident-output/internal/wire"
)

// ErrFileConflictingProducer is returned when two Tasks in one Run both
// claim the same canonical File output path (spec §8.3/§11.4: "more than
// one producing operation claiming the same canonical file output in one
// Run is a conflict").
var ErrFileConflictingProducer = errors.New("evo: File output path already claimed by another Task in this Run")

// manifestFor returns this Run's manifest Store, opening it on first use
// (spec §11.3) — the same lazy-capture pattern workspaceDirLocked already
// uses for the workspace directory. Every later call, whether it succeeded
// or failed, returns the same cached result: a manifest miss/open failure
// degrades this Run to live-filesystem-only File behavior rather than
// retrying on every call.
func (o *Output) manifestFor(ctx context.Context) (*manifest.Store, error) {
	workspace := o.workspaceDirLocked()
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.manifestOpened {
		return o.manifestStore, o.manifestOpenErr
	}
	o.manifestOpened = true
	cfg := manifest.Config{AppID: o.cfg.appID, StateDir: o.cfg.stateDir, Workspace: workspace}
	store, err := manifest.Open(ctx, cfg, manifest.NewOSEnvironment())
	o.manifestStore = store
	o.manifestOpenErr = err
	if err == nil {
		o.manifestApp = manifest.ApplicationRecord{ID: o.cfg.appID}
		if appFP, appErr := fingerprint.App().Fingerprint(ctx); appErr == nil {
			o.manifestApp.Fingerprint = "sha256:" + hex.EncodeToString(appFP.Digest[:])
		}
		o.manifestAppDone = true
	}
	return store, err
}

// emitManifestWarningOnce surfaces a manifest Store's safe cache-miss
// warning (corrupt/unknown prior manifest, spec §11.3) as a run-scoped Fact
// exactly once per Run — never as an error, since a miss is always safe to
// continue past.
func (o *Output) emitManifestWarningOnce(w error) {
	if w == nil {
		return
	}
	o.mu.Lock()
	if o.manifestWarningIssued {
		o.mu.Unlock()
		return
	}
	o.manifestWarningIssued = true
	o.mu.Unlock()
	o.Fact("manifest", w.Error())
}

// claimManifestOutputLocked records that taskID is the producing Task for
// canonical path, or reports ErrFileConflictingProducer when a different
// Task already claimed it in this Run (spec §8.3/§11.4). Callers must
// already hold o.mu.
func (o *Output) claimManifestOutputLocked(taskID, path string) error {
	if o.manifestClaims == nil {
		o.manifestClaims = make(map[string]string)
	}
	if owner, claimed := o.manifestClaims[path]; claimed && owner != taskID {
		return fmt.Errorf("%w: %s", ErrFileConflictingProducer, path)
	}
	o.manifestClaims[path] = taskID
	return nil
}

// taskManifestKeyLocked returns taskID's stable manifest Task key (§3.1's
// stableKey, already computed at declaration) and the next pending
// operation ordinal within it. Callers must already hold o.mu.
func (o *Output) taskManifestKeyLocked(taskID string) (key string, ordinal int, ok bool) {
	st := o.taskByRef[taskID]
	if st == nil {
		return "", 0, false
	}
	return st.key, len(st.manifestOps), true
}

// appendManifestOperationLocked records rec as taskID's next pending
// operation, committed only if/when the Task itself settles Done (spec
// §8.2/§11.3). Callers must already hold o.mu.
func (o *Output) appendManifestOperationLocked(taskID string, rec manifest.OperationRecord) {
	if st := o.taskByRef[taskID]; st != nil {
		st.manifestOps = append(st.manifestOps, rec)
	}
}

// commitManifestTaskLocked persists taskID's accumulated operations as this
// Run's truth (spec §11.3) once the Task has settled Done. A Task that
// recorded no tracked operations, or a Run with no usable manifest Store,
// commits nothing. Callers must already hold o.mu.
func (o *Output) commitManifestTaskLocked(ctx context.Context, taskID string) {
	st := o.taskByRef[taskID]
	if st == nil || len(st.manifestOps) == 0 || o.manifestStore == nil {
		return
	}
	task := manifest.TaskRecord{Key: st.key, Operations: append([]manifest.OperationRecord(nil), st.manifestOps...)}
	if err := o.manifestStore.CommitTask(ctx, o.manifestApp, task); err == nil {
		o.emitWireEventLocked(wire.EventManifestTaskCommitted, taskID, map[string]any{
			"operations": len(task.Operations),
		})
	}
}

// fileBasisRecords fingerprints every entry in basis (spec §11.1) and
// returns them canonicalized by (Kind, Key) — Basis order is semantically
// irrelevant (§11.1). A duplicate (Kind, Key) pair is a programmer error
// (§11.1).
func fileBasisRecords(ctx context.Context, basis []fingerprint.Fingerprint) ([]manifest.BasisRecord, error) {
	records := make([]manifest.BasisRecord, 0, len(basis))
	for _, b := range basis {
		v, err := b.Fingerprint(ctx)
		if err != nil {
			return nil, fmt.Errorf("evo: File Basis: %w", err)
		}
		records = append(records, manifest.BasisRecord{
			Kind:   string(v.Kind),
			Key:    v.Key,
			Digest: hex.EncodeToString(v.Digest[:]),
		})
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].Kind != records[j].Kind {
			return records[i].Kind < records[j].Kind
		}
		return records[i].Key < records[j].Key
	})
	for i := 1; i < len(records); i++ {
		if records[i].Kind == records[i-1].Kind && records[i].Key == records[i-1].Key {
			return nil, fmt.Errorf("evo: File Basis: duplicate (kind=%s, key=%s)", records[i].Kind, records[i].Key)
		}
	}
	return records, nil
}

// fileDefinitionFingerprint computes File's operation definition fingerprint
// (spec §11.4): canonical path + managed contents digest + managed mode +
// sorted Basis descriptors. basis must already be canonicalized (see
// fileBasisRecords) so two equivalent Basis sets always hash identically.
func fileDefinitionFingerprint(path string, contentsManaged bool, contents []byte, mode uint32, basis []manifest.BasisRecord) string {
	h := sha256.New()
	_, _ = h.Write([]byte("evident-output:file:definition:v1\x00"))
	_, _ = h.Write([]byte(path))
	h.Write([]byte{0})
	if contentsManaged {
		sum := sha256.Sum256(contents)
		_, _ = h.Write([]byte("contents:managed\x00"))
		h.Write(sum[:])
	} else {
		_, _ = h.Write([]byte("contents:unmanaged\x00"))
	}
	_, _ = fmt.Fprintf(h, "mode:%d\x00", mode)
	for _, b := range basis {
		_, _ = h.Write([]byte(b.Kind))
		h.Write([]byte{0})
		_, _ = h.Write([]byte(b.Key))
		h.Write([]byte{0})
		_, _ = h.Write([]byte(b.Digest))
		h.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// fileOutputDigest fingerprints path's current on-disk content (spec
// §8.3/§11.1) for storage as, or comparison against, a File operation's
// tracked output record.
func fileOutputDigest(ctx context.Context, path string) (string, error) {
	v, err := fingerprint.FSPath(path).Fingerprint(ctx)
	if err != nil {
		return "", fmt.Errorf("evo: File output %q: %w", path, err)
	}
	return hex.EncodeToString(v.Digest[:]), nil
}

// Reasons fileOperationCurrent reports for a "not current" verdict (spec
// §38: events must distinguish Basis drift from tracked output drift from
// no prior record at all).
const (
	freshnessReasonNoPriorRecord   = "no_prior_record"
	freshnessReasonBasisDrift      = "basis_drift"
	freshnessReasonDefinitionDrift = "definition_changed"
	freshnessReasonTrackedDrift    = "tracked_output_drift"
	freshnessReasonCurrent         = "current"
)

// fileOperationCurrent reports whether prior (the previously committed
// operation record for this Task's Nth File call, if any) still matches:
// every Basis digest, the operation definition itself, and the tracked
// output's current on-disk digest (spec §8.2/§11.4/§11.5). A prior record's
// absence, a Basis/definition mismatch, or output drift (edited outside
// Evo) are all "not current" — never an error on their own. reason names
// which of those applied, or freshnessReasonCurrent when isCurrent is true.
// Basis is checked ahead of the combined DefinitionFingerprint (which
// itself already hashes Basis in, see fileDefinitionFingerprint) so a
// Basis-only change reports freshnessReasonBasisDrift rather than being
// folded into the more generic definition mismatch (spec §38: "Basis
// drift" and "tracked output drift" must be distinguishable events).
func fileOperationCurrent(ctx context.Context, prior manifest.OperationRecord, hasPrior bool, defFingerprint string, basis []manifest.BasisRecord, path string) (isCurrent bool, reason string, err error) {
	if !hasPrior || len(prior.Outputs) != 1 {
		return false, freshnessReasonNoPriorRecord, nil
	}
	if !basisRecordsEqual(prior.Basis, basis) {
		return false, freshnessReasonBasisDrift, nil
	}
	if prior.DefinitionFingerprint != defFingerprint {
		return false, freshnessReasonDefinitionDrift, nil
	}
	liveDigest, err := fileOutputDigest(ctx, path)
	if err != nil {
		return false, "", err
	}
	if prior.Outputs[0].Digest != liveDigest {
		return false, freshnessReasonTrackedDrift, nil
	}
	return true, freshnessReasonCurrent, nil
}

// basisRecordsEqual compares two already-canonicalized Basis slices
// element-wise (see fileBasisRecords) — canonicalization makes a
// straightforward positional comparison correct rather than needing its
// own set-equality pass.
func basisRecordsEqual(a, b []manifest.BasisRecord) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
