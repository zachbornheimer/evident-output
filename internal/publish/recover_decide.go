package publish

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	sysfs "github.com/zachbornheimer/evident-output/internal/fs"
)

// leftover is a staging entry beside dest. tree is its digest, "" when it
// is live, not a real directory, or cannot be digested; lease is held
// while Recover decides, so no writer can start using its name.
type leftover struct {
	path, tree string
	live       bool
	lease      *stageLease
}

type leftovers []leftover

// inspectLeftovers leases and digests dest's leftovers. One whose lease a
// live writer holds is marked live and never digested.
func inspectLeftovers(ctx context.Context, dest string, ev Evidence) (leftovers, error) {
	entries, err := Leftovers(dest)
	if err != nil {
		return nil, err
	}
	found := make(leftovers, len(entries))
	for i, path := range entries {
		found[i].path = path
		lease, ok, err := tryLease(path)
		if err != nil {
			return found, errors.Join(err, found.release())
		}
		if !ok {
			found[i].live = true
			continue
		}
		found[i].lease = lease
		if tree, err := treeDigest(ctx, path, ev); err == nil {
			found[i].tree = tree
		}
	}
	return found, nil
}

func (found leftovers) release() error {
	var errs []error
	for _, l := range found {
		errs = append(errs, l.lease.release())
	}
	return errors.Join(errs...)
}

func (found leftovers) paths() []string {
	out := make([]string, len(found))
	for i, l := range found {
		out[i] = l.path
	}
	return out
}

// decision is what Recover will do: the outcome, the leftover to restore
// to dest (if any), the ones to delete, and the ones it keeps.
type decision struct {
	outcome Outcome
	restore string
	remove  []string
	kept    []string
}

// decide settles dest from ev's digests without changing anything.
func decide(ctx context.Context, dest string, ev Evidence, found leftovers) (decision, error) {
	d := decision{outcome: OutcomeUnrecoverable, kept: found.paths()}
	tree, err := treeDigest(ctx, dest, ev)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		d.restore = found.original(ev)
		if d.restore == "" {
			return d, fmt.Errorf("%w: %s is absent and no leftover digests to the original", ErrUnrecoverable, dest)
		}
		d.outcome = OutcomeRestoredOriginal
	case err != nil:
		return d, fmt.Errorf("%w: %w", ErrUnrecoverable, err)
	case ev.isOriginal(tree):
		d.outcome = OutcomeIntact
	case ev.isReplacement(tree):
		d.outcome = OutcomeCompletedReplacement
	default:
		return d, fmt.Errorf("%w: %s digests to %s, neither the original nor the replacement", ErrUnrecoverable, dest, tree)
	}
	d.kept = nil
	for _, l := range found {
		switch {
		case l.path == d.restore:
		case !l.live && (ev.isOriginal(l.tree) || ev.isReplacement(l.tree)):
			d.remove = append(d.remove, l.path)
		default:
			d.kept = append(d.kept, l.path)
		}
	}
	return d, nil
}

// original is the first leftover that provably holds ev.Original.
func (found leftovers) original(ev Evidence) string {
	for _, l := range found {
		if !l.live && ev.isOriginal(l.tree) {
			return l.path
		}
	}
	return ""
}

// apply carries d out: restore and re-verify the original, then delete
// what d proved redundant. A failed restore deletes nothing.
func (d decision) apply(ctx context.Context, dest string, ev Evidence) (Recovery, error) {
	if d.restore != "" {
		if err := restoreOriginal(ctx, dest, d.restore, ev); err != nil {
			return Recovery{Outcome: OutcomeUnrecoverable, Kept: append(d.kept, d.remove...)}, err
		}
	}
	kept := d.kept
	var errs []error
	for _, path := range d.remove {
		if err := removeReplaced(path); err != nil {
			kept = append(kept, path)
			errs = append(errs, err)
		}
	}
	return Recovery{Outcome: d.outcome, Kept: kept}, errors.Join(errs...)
}

// restoreOriginal renames the leftover at from back to the absent dest and
// re-verifies it there.
func restoreOriginal(ctx context.Context, dest, from string, ev Evidence) error {
	if err := sysfs.Rename(from, dest); err != nil {
		return fmt.Errorf("restore original from %s: %w", from, err)
	}
	tree, err := treeDigest(ctx, dest, ev)
	if err != nil {
		return fmt.Errorf("verify restored original at %s: %w", dest, err)
	}
	if !ev.isOriginal(tree) {
		return fmt.Errorf("restored original at %s digests to %s, not %s", dest, tree, ev.Original)
	}
	return nil
}

// treeDigest digests the real directory at path: fs.ErrNotExist when it is
// absent, an error for anything that is not a directory.
func treeDigest(ctx context.Context, path string, ev Evidence) (string, error) {
	info, err := sysfs.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory (%s)", path, info.Mode().Type())
	}
	return ev.Digest(ctx, path)
}
