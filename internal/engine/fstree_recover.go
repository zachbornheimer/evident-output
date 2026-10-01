package engine

import (
	"context"
	"errors"
	"fmt"

	"github.com/zachbornheimer/evident-output/internal/checksum"
	"github.com/zachbornheimer/evident-output/internal/publish"
)

// TreeRecover is Tree.Recover: it settles path after a Replace of src over
// expected was interrupted (see publish.Recover). It runs only inside a
// Task; under DryRun it reports the decision and changes nothing. src may
// be nil: the replacement is then unknown.
func TreeRecover(ctx context.Context, path string, src TreeSource, expected string) (publish.Recovery, error) {
	label := fmt.Sprintf("Tree %q Recover", path)
	unrecoverable := publish.Recovery{Outcome: publish.OutcomeUnrecoverable}
	task, err := beginOperation(ctx, label)
	if err != nil {
		return unrecoverable, err
	}
	if path == "" {
		return unrecoverable, ErrPathMissing
	}
	out := task.out
	abs := out.checksumPath(path)
	replacement, err := out.sourceDigest(ctx, abs, src)
	if err != nil {
		return unrecoverable, fmt.Errorf("evo: %s: %w", label, err)
	}
	ev := publish.Evidence{
		Original:    expected,
		Replacement: replacement,
		Digest: func(ctx context.Context, path string) (string, error) {
			digest, err := out.treeDigest(ctx, path, checksum.Exclusion{})
			return digest.String(), err
		},
	}
	settle := publish.Recover
	if out.DryRun() {
		settle = publish.PlanRecover
	}
	got, err := settle(ctx, abs, ev)
	switch {
	case errors.Is(err, publish.ErrUnrecoverable):
		return got, fmt.Errorf("evo: %s: %w: %w", label, ErrTreeChanged, err)
	case err != nil:
		return got, fmt.Errorf("evo: %s: %w", label, err)
	}
	return got, nil
}

// sourceDigest is the digest src's tree would have, or "" for no src. It
// is prepared beside path, as Verify does, and always discarded.
func (o *Output) sourceDigest(ctx context.Context, path string, src TreeSource) (string, error) {
	if src == nil {
		return "", nil
	}
	staged, err := publish.StageTree(ctx, path, 0, src.Fill)
	if err != nil {
		return "", err
	}
	digest, err := o.treeDigest(ctx, staged.Path(), checksum.Exclusion{})
	if discardErr := staged.Discard(); err == nil && discardErr != nil {
		return "", discardErr
	}
	if err != nil {
		return "", fmt.Errorf("digest prepared tree: %w", err)
	}
	return digest.String(), nil
}
