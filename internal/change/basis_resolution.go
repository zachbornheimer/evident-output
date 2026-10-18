// This file owns how a tracked operation's Basis becomes canonical: one
// step shared by Exec and File so fingerprinting, the dry-run plan and the
// freshness barrier all agree on one path per input.

package change

import (
	"context"
	"fmt"

	"github.com/zachbornheimer/evident-output/v2/internal/freshness"
)

// resolvedBasisRecords fingerprints spec.Basis for one Exec or File call (spec §11.1,
// §11.6): an FSPath entry is first resolved
// against the workspace (spec §8.4: "FSPath Basis against workspace") and
// awaited on the freshness barrier for that canonical path — so a Basis
// input this Run's own Exec/File output claims is always fingerprinted only
// after its producing operation has settled (§11.6), never mid-write. Each
// FSPath entry is then observed under a read claim (ObserveBasis), which
// also excludes File commits from other Runs.
func (r *Run) resolvedBasisRecords(ctx context.Context, basis []freshness.Fingerprint) ([]freshness.BasisRecord, error) {
	resolved := make([]freshness.Fingerprint, len(basis))
	for i, b := range basis {
		path, isPath := freshness.PathOf(b)
		if !isPath {
			resolved[i] = b
			continue
		}
		canon := r.resolvePath(path)
		if awaitErr := r.barrier.Await(ctx, canon); awaitErr != nil {
			return nil, fmt.Errorf("evo: Basis: %w", awaitErr)
		}
		resolved[i] = freshness.FSPath(canon)
	}
	return r.ObserveBasis(ctx, resolved)
}
