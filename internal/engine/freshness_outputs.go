package engine

import (
	"context"
	"fmt"

	"github.com/zachbornheimer/evident-output/internal/manifest"
)

// execOutputsOperationKind marks the manifest record of the state an Exec
// produced. A Task whose Basis is unchanged is current only while every
// such recorded Output still has the identity the last Run observed.
const execOutputsOperationKind = "exec_outputs"

// recordExecOutputs observes each declared Output after a verified run and
// records its identity with the Task, committed only if the Task succeeds.
func (o *Output) recordExecOutputs(ctx context.Context, taskID, dir string, outputs []ExecOutput) error {
	if len(outputs) == 0 {
		return nil
	}
	records := make([]manifest.OutputRecord, 0, len(outputs))
	for _, out := range outputs {
		rec, err := o.observeOutput(ctx, outputKind(out), resolvePathAgainst(dir, out.Path))
		if err != nil {
			return err
		}
		records = append(records, rec)
	}
	o.mu.Lock()
	o.appendManifestOperationLocked(taskID, manifest.OperationRecord{Kind: execOutputsOperationKind, Outputs: records})
	o.mu.Unlock()
	return nil
}

func outputKind(out ExecOutput) string {
	if out.Tree {
		return basisKindTree
	}
	return basisKindFile
}

// observeOutput is an Output's identity, computed exactly as a Basis input
// of the same kind is, so the two never disagree about one path.
func (o *Output) observeOutput(ctx context.Context, kind, abs string) (manifest.OutputRecord, error) {
	digest, err := o.fileBasisDigest(ctx, abs)
	if kind == basisKindTree {
		digest, err = o.treeBasisDigest(ctx, abs)
	}
	if err != nil {
		return manifest.OutputRecord{}, fmt.Errorf("observe Output %q: %w", abs, err)
	}
	return manifest.OutputRecord{Kind: kind, Path: abs, Digest: digest}, nil
}

// recordedOutputsHold reports whether every Exec Output recorded by the
// Task's last successful Run still has the identity recorded then. An
// Output that can no longer be observed does not hold.
func (o *Output) recordedOutputsHold(ctx context.Context, operations []manifest.OperationRecord) (bool, error) {
	for _, op := range operations {
		if op.Kind != execOutputsOperationKind {
			continue
		}
		for _, rec := range op.Outputs {
			now, err := o.observeOutput(ctx, rec.Kind, rec.Path)
			if err != nil || now.Digest != rec.Digest {
				return false, ctx.Err()
			}
		}
	}
	return true, nil
}
