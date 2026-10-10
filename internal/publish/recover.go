package publish

import (
	"context"
	"errors"
	"fmt"

	sysfs "github.com/zachbornheimer/evident-output/internal/fs"
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
	// proven whole, or owned by a live writer.
	Kept []string
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
// A leftover a live writer still owns (see stageLease), or that is not a
// real directory, is never restored or deleted.
func Recover(ctx context.Context, dest string, ev Evidence) (Recovery, error) {
	return settle(ctx, dest, ev, true)
}

// PlanRecover is Recover deciding without changing anything: the Recovery
// it reports is what Recover would do now.
func PlanRecover(ctx context.Context, dest string, ev Evidence) (Recovery, error) {
	return settle(ctx, dest, ev, false)
}

func settle(ctx context.Context, dest string, ev Evidence, apply bool) (Recovery, error) {
	abs, err := sysfs.Abs(dest)
	if err != nil {
		return Recovery{Outcome: OutcomeUnrecoverable}, fmt.Errorf("publish: recover %s: %w", dest, err)
	}
	hold, err := Lock(ctx, abs)
	if err != nil {
		return Recovery{Outcome: OutcomeUnrecoverable}, fmt.Errorf("publish: recover %s: %w", abs, err)
	}
	found, err := inspectLeftovers(ctx, abs, ev)
	var r Recovery
	if err == nil {
		r, err = settleLocked(ctx, abs, ev, found, apply)
	} else {
		r = Recovery{Outcome: OutcomeUnrecoverable}
	}
	if err := errors.Join(err, found.release(), hold.Release()); err != nil {
		return r, fmt.Errorf("publish: recover %s: %w", abs, err)
	}
	return r, nil
}

func settleLocked(ctx context.Context, dest string, ev Evidence, found leftovers, apply bool) (Recovery, error) {
	d, err := decide(ctx, dest, ev, found)
	if err != nil || !apply {
		return Recovery{Outcome: d.outcome, Kept: d.kept}, err
	}
	return d.apply(ctx, dest, ev)
}
