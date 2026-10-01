package publish

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ErrUnrecoverable is a Recover that could prove neither planned tree at
// the destination nor restore the original: nothing was changed.
var ErrUnrecoverable = errors.New("publish: no planned tree to recover")

// Evidence is what an interrupted tree commit planned with: the only basis
// Recover decides on. A leftover's name, age, or presence proves nothing.
type Evidence struct {
	// Original is the digest the destination held when the commit was
	// planned (Replace's expected).
	Original string
	// Replacement is the digest of the tree being published, or "" when
	// the caller cannot say.
	Replacement string
	// Digest is the tree digest of the directory at path.
	Digest func(ctx context.Context, path string) (string, error)
}

// isOriginal reports whether a tree digest is the planned original.
func (e Evidence) isOriginal(tree string) bool { return sameTree(tree, e.Original) }

// isReplacement reports whether a tree digest is the planned replacement.
func (e Evidence) isReplacement(tree string) bool { return sameTree(tree, e.Replacement) }

// sameTree reports whether two content digests name one known tree. An
// unknown ("") digest matches nothing. Digests identify public content and
// guard no secret.
func sameTree(a, b string) bool { return a != "" && a == b }

// Outcome is how Recover settled a destination.
type Outcome int

const (
	// OutcomeIntact: the destination holds the original.
	OutcomeIntact Outcome = iota + 1
	// OutcomeRestoredOriginal: the destination was absent and the original
	// was put back from a leftover.
	OutcomeRestoredOriginal
	// OutcomeCompletedReplacement: the destination holds the replacement.
	OutcomeCompletedReplacement
	// OutcomeUnrecoverable: neither planned tree could be established;
	// nothing was changed.
	OutcomeUnrecoverable
)

// Recovery is Recover's result.
type Recovery struct {
	Outcome Outcome
	// Kept are leftovers left in place: not proven to be a planned tree,
	// or (when Unrecoverable) not deleted because the destination is not
	// proven whole.
	Kept []string
}

// leftover is a staging entry beside dest and what it digests to; tree is
// "" for one that is not a real directory or cannot be digested.
type leftover struct {
	path, tree string
}

// Recover settles dest after an interrupted tree commit, under dest's
// coordination, deciding by ev's digests alone:
//
//   - dest digests to ev.Original or ev.Replacement: dest is the whole
//     truth (Intact or CompletedReplacement); leftovers digesting to a
//     planned tree are deleted, every other leftover is kept.
//   - dest is absent and a leftover digests to ev.Original: that leftover
//     is renamed back to dest and re-verified (RestoredOriginal), then
//     planned leftovers are deleted.
//   - anything else (dest foreign, not a directory, or absent with no
//     original to restore): Unrecoverable, ErrUnrecoverable, and nothing
//     is changed.
//
// A leftover that is not a real directory is never restored or deleted.
func Recover(ctx context.Context, dest string, ev Evidence) (Recovery, error) {
	abs, err := filepath.Abs(dest)
	if err != nil {
		return Recovery{Outcome: OutcomeUnrecoverable}, fmt.Errorf("publish: recover %s: %w", dest, err)
	}
	hold, err := Lock(ctx, abs)
	if err != nil {
		return Recovery{Outcome: OutcomeUnrecoverable}, fmt.Errorf("publish: recover %s: %w", abs, err)
	}
	r, recoverErr := recoverLocked(ctx, abs, ev)
	if err := errors.Join(recoverErr, hold.Release()); err != nil {
		return r, fmt.Errorf("publish: recover %s: %w", abs, err)
	}
	return r, nil
}

func recoverLocked(ctx context.Context, dest string, ev Evidence) (Recovery, error) {
	found, err := digestLeftovers(ctx, dest, ev)
	if err != nil {
		return Recovery{Outcome: OutcomeUnrecoverable}, err
	}
	outcome, err := settleDest(ctx, dest, ev, found)
	if err != nil {
		return Recovery{Outcome: OutcomeUnrecoverable, Kept: paths(found)}, err
	}
	kept, err := removePlanned(ev, found)
	return Recovery{Outcome: outcome, Kept: kept}, err
}

// settleDest proves dest is a planned tree, restoring the original over an
// absent dest when a leftover is provably it.
func settleDest(ctx context.Context, dest string, ev Evidence, found []leftover) (Outcome, error) {
	tree, err := treeDigest(ctx, dest, ev)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return restoreOriginal(ctx, dest, ev, found)
	case err != nil:
		return 0, fmt.Errorf("%w: %w", ErrUnrecoverable, err)
	case ev.isOriginal(tree):
		return OutcomeIntact, nil
	case ev.isReplacement(tree):
		return OutcomeCompletedReplacement, nil
	}
	return 0, fmt.Errorf("%w: %s digests to %s, neither the original nor the replacement", ErrUnrecoverable, dest, tree)
}

// restoreOriginal renames the leftover that digests to ev.Original back to
// the absent dest, and re-verifies it there.
func restoreOriginal(ctx context.Context, dest string, ev Evidence, found []leftover) (Outcome, error) {
	for _, l := range found {
		if !ev.isOriginal(l.tree) {
			continue
		}
		if err := os.Rename(l.path, dest); err != nil {
			return 0, fmt.Errorf("restore original from %s: %w", l.path, err)
		}
		tree, err := treeDigest(ctx, dest, ev)
		if err != nil {
			return 0, fmt.Errorf("verify restored original at %s: %w", dest, err)
		}
		if !ev.isOriginal(tree) {
			return 0, fmt.Errorf("restored original at %s digests to %s, not %s", dest, tree, ev.Original)
		}
		return OutcomeRestoredOriginal, nil
	}
	return 0, fmt.Errorf("%w: %s is absent and no leftover digests to the original", ErrUnrecoverable, dest)
}

// removePlanned deletes the leftovers that are planned trees, now that
// dest is proven whole, and returns the ones it kept.
func removePlanned(ev Evidence, found []leftover) ([]string, error) {
	var kept []string
	var errs []error
	for _, l := range found {
		if _, err := os.Lstat(l.path); errors.Is(err, fs.ErrNotExist) {
			continue // the leftover restored to dest
		}
		if !ev.isOriginal(l.tree) && !ev.isReplacement(l.tree) {
			kept = append(kept, l.path)
			continue
		}
		if err := removeReplaced(l.path); err != nil {
			kept = append(kept, l.path)
			errs = append(errs, err)
		}
	}
	return kept, errors.Join(errs...)
}

// digestLeftovers lists dest's leftovers with their digests. Only a real
// directory is digested, so a symlinked leftover can never pass for a
// planned tree.
func digestLeftovers(ctx context.Context, dest string, ev Evidence) ([]leftover, error) {
	entries, err := Leftovers(dest)
	if err != nil {
		return nil, err
	}
	found := make([]leftover, len(entries))
	for i, path := range entries {
		found[i].path = path
		if tree, err := treeDigest(ctx, path, ev); err == nil {
			found[i].tree = tree
		}
	}
	return found, nil
}

// treeDigest digests the real directory at path: fs.ErrNotExist when it is
// absent, an error for anything that is not a directory.
func treeDigest(ctx context.Context, path string, ev Evidence) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory (%s)", path, info.Mode().Type())
	}
	return ev.Digest(ctx, path)
}

func paths(found []leftover) []string {
	out := make([]string, len(found))
	for i, l := range found {
		out[i] = l.path
	}
	return out
}
