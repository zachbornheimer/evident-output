package engine

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/zachbornheimer/evident-output/internal/checksum"
	"github.com/zachbornheimer/evident-output/internal/fingerprint"
	"github.com/zachbornheimer/evident-output/internal/manifest"
	"github.com/zachbornheimer/evident-output/internal/wire"
)

const (
	// taskBasisOperationKind marks the manifest record that carries a
	// Task's observed Basis. It is always the last operation of the Task.
	taskBasisOperationKind = "task_basis"

	basisKindFile = "fs_file"
	basisKindTree = "fs_tree"

	// basisDigestMissing is the identity of a path that does not exist, so
	// a Basis input may name something not created yet.
	basisDigestMissing = "missing"
)

// BasisSource is one Task Basis input: something whose content identity is
// observed when the Task starts. Build one with FileBasis, TreeBasis, or
// FingerprintBasis.
type BasisSource interface {
	observe(ctx context.Context, o *Output) (manifest.BasisRecord, error)
}

// FileBasis observes the file at path by content.
func FileBasis(path string) BasisSource { return fileBasis{path: path} }

// TreeBasis observes the directory tree at path by content and structure.
func TreeBasis(path string) BasisSource { return treeBasis{path: path} }

// FingerprintBasis observes a non-filesystem Fingerprint.
func FingerprintBasis(f fingerprint.Fingerprint) BasisSource { return fingerprintBasis{inner: f} }

type fileBasis struct{ path string }
type treeBasis struct{ path string }
type fingerprintBasis struct{ inner fingerprint.Fingerprint }

func (b fileBasis) observe(ctx context.Context, o *Output) (manifest.BasisRecord, error) {
	if b.path == "" {
		return manifest.BasisRecord{}, ErrPathMissing
	}
	abs := o.checksumPath(b.path)
	digest, err := o.fileBasisDigest(ctx, abs)
	if err != nil {
		return manifest.BasisRecord{}, fmt.Errorf("evo: Basis File %q: %w", b.path, err)
	}
	return manifest.BasisRecord{Kind: basisKindFile, Key: abs, Digest: digest}, nil
}

func (b treeBasis) observe(ctx context.Context, o *Output) (manifest.BasisRecord, error) {
	if b.path == "" {
		return manifest.BasisRecord{}, ErrPathMissing
	}
	abs := o.checksumPath(b.path)
	digest, err := o.treeBasisDigest(ctx, abs)
	if err != nil {
		return manifest.BasisRecord{}, fmt.Errorf("evo: Basis Tree %q: %w", b.path, err)
	}
	return manifest.BasisRecord{Kind: basisKindTree, Key: abs, Digest: digest}, nil
}

func (b fingerprintBasis) observe(ctx context.Context, _ *Output) (manifest.BasisRecord, error) {
	records, err := basisRecordsFrom(ctx, []fingerprint.Fingerprint{b.inner})
	if err != nil {
		return manifest.BasisRecord{}, err
	}
	return records[0], nil
}

// fileBasisDigest is a file's Basis identity: its content digest from the
// checksum engine, "missing" when absent, and for a symlink the link text
// plus the digest of what it resolves to. A directory or an unreadable file
// is an error, never "unchanged".
func (o *Output) fileBasisDigest(ctx context.Context, abs string) (string, error) {
	info, err := o.fileFSOrDefault().Lstat(abs)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return basisDigestMissing, nil
	case err != nil:
		return "", err
	case info.Mode()&fs.ModeSymlink != 0:
		return o.symlinkBasisDigest(ctx, abs)
	}
	digest, err := o.fileDigest(ctx, abs)
	if err != nil {
		return "", err
	}
	return digest.String(), nil
}

func (o *Output) symlinkBasisDigest(ctx context.Context, abs string) (string, error) {
	target, err := os.Readlink(abs)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if errors.Is(err, fs.ErrNotExist) {
		return "symlink:" + target + ":dangling", nil
	}
	if err != nil {
		return "", err
	}
	content, err := o.fileDigest(ctx, resolved)
	if err != nil {
		return "", err
	}
	return "symlink:" + target + ":" + content.String(), nil
}

func (o *Output) treeBasisDigest(ctx context.Context, abs string) (string, error) {
	if _, err := o.fileFSOrDefault().Lstat(abs); errors.Is(err, fs.ErrNotExist) {
		return basisDigestMissing, nil
	}
	digest, err := o.treeDigest(ctx, abs, checksum.Exclusion{})
	if errors.Is(err, fs.ErrNotExist) {
		return basisDigestMissing, nil
	}
	if err != nil {
		return "", err
	}
	return digest.String(), nil
}

// observeTaskBasis observes every input and canonicalizes the result. Basis
// order is not identity; a repeated (kind, key) is a programmer error.
func (o *Output) observeTaskBasis(ctx context.Context, inputs []BasisSource) ([]manifest.BasisRecord, error) {
	records := make([]manifest.BasisRecord, 0, len(inputs))
	for _, in := range inputs {
		rec, err := in.observe(ctx, o)
		if err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	sortBasisRecords(records)
	for i := 1; i < len(records); i++ {
		if records[i].Kind == records[i-1].Kind && records[i].Key == records[i-1].Key {
			return nil, fmt.Errorf("evo: Basis: duplicate (kind=%s, key=%s)", records[i].Kind, records[i].Key)
		}
	}
	return records, nil
}

// basisIsCurrent observes this Task's Basis as it starts and reports
// whether the Task's last successful Run recorded the same identity. The
// observation holds no resource claim, so it never orders or blocks any
// other Task. A Task with no Basis is never current. Without a prior
// record the Task runs.
func (t *TaskHandle) basisIsCurrent(ctx context.Context) (bool, error) {
	o := t.out
	o.mu.Lock()
	st := o.taskByRef[t.id]
	var inputs []BasisSource
	if st != nil {
		inputs = append(inputs, st.basisInputs...)
	}
	o.mu.Unlock()
	if len(inputs) == 0 {
		return false, nil
	}
	observed, err := o.observeTaskBasis(ctx, inputs)
	if err != nil {
		return false, err
	}
	store, err := o.manifestFor(ctx)
	if err != nil {
		return false, fmt.Errorf("evo: Basis: %w", err)
	}
	o.mu.Lock()
	prior, ok := store.Task(st.key)
	o.mu.Unlock()
	operations, priorBasis := splitBasisOperation(prior.Operations)
	current := ok && priorBasis != nil && basisRecordsEqual(priorBasis, observed)
	if current {
		if current, err = o.recordedOutputsHold(ctx, operations); err != nil {
			return false, err
		}
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	st.basisObserved = observed
	o.emitWireEventLocked(wire.EventBasisFingerprinted, t.id, map[string]any{
		"count": len(observed), "current": current,
	})
	if current {
		st.manifestOps = operations
	}
	return current, nil
}

// splitBasisOperation separates a committed task's trailing Basis record
// from its other operations. basis is nil when the task recorded none.
func splitBasisOperation(ops []manifest.OperationRecord) (rest []manifest.OperationRecord, basis []manifest.BasisRecord) {
	if n := len(ops); n > 0 && ops[n-1].Kind == taskBasisOperationKind {
		return append([]manifest.OperationRecord(nil), ops[:n-1]...), ops[n-1].Basis
	}
	return append([]manifest.OperationRecord(nil), ops...), nil
}

// operationsToCommit is the task's operations plus, when it declared a
// Basis, the Basis record last. Callers must hold o.mu.
func (st *taskState) operationsToCommit() []manifest.OperationRecord {
	ops := append([]manifest.OperationRecord(nil), st.manifestOps...)
	if st.basisObserved != nil {
		ops = append(ops, manifest.OperationRecord{Kind: taskBasisOperationKind, Basis: st.basisObserved})
	}
	return ops
}
