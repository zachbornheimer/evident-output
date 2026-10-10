package engine

import (
	"context"

	"github.com/zachbornheimer/evident-output/internal/freshness"
)

// recordedOutputsHold reports whether every Output recorded by the Task's
// last successful Run still has the identity recorded then. An Output that
// can no longer be observed does not hold.
func (o *Output) recordedOutputsHold(ctx context.Context, operations []freshness.OperationRecord) (bool, error) {
	for _, op := range operations {
		for _, rec := range op.Outputs {
			digest, err := pathOutputDigest(ctx, rec.Path)
			if err != nil || digest != rec.Digest {
				return false, ctx.Err()
			}
		}
	}
	return true, nil
}
