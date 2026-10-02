package publish

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/zachbornheimer/evident-output/internal/publish/stagename"
)

// Staging entries are prepared apart from the destination, in the staging
// root on its volume (see stagingDirFor), and become private siblings of
// it (see stagename) under its lock, so the final rename never crosses a
// filesystem.
const (
	// stagingAttempts bounds the search for an unused staging name.
	stagingAttempts = 100
	// defaultTreeMode is a staged tree root's mode when none is given.
	defaultTreeMode fs.FileMode = 0o755
	// parentDirMode is the mode of parent directories staging creates.
	parentDirMode fs.FileMode = 0o755
	// stagingFileMode keeps a staged file private until its final chmod.
	stagingFileMode fs.FileMode = 0o600
	// stagingDirMode keeps a staged tree private until its final chmod.
	stagingDirMode fs.FileMode = 0o700
)

// ErrSpent is a Staged whose content was already committed or discarded.
var ErrSpent = errors.New("publish: staged content already committed or discarded")

// IsStaging reports whether name is exactly a publish staging entry.
// Readers that enumerate a directory (Find, Tree.Read) and tree digests
// skip these: they are another writer's uncommitted state, never part of
// the tree.
func IsStaging(name string) bool { return stagename.Is(name) }

// Staged is content prepared for its destination and not yet visible
// there. Commit publishes it; Discard abandons it. Exactly one of the two
// must run, and either leaves nothing staged behind.
type Staged struct {
	dest    string
	temp    string
	tree    bool
	apart   bool     // temp is in the staging root, not yet beside dest
	created []string // parent directories staging made, outermost first
	spent   bool
	lease   *stageLease
}

// own leases the fresh staging entry s.temp for this writer.
func (s *Staged) own() error {
	lease, ok, err := tryLease(s.temp)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("staging entry %s is already leased", s.temp)
	}
	s.lease = lease
	return nil
}

// Dest is the absolute destination path.
func (s *Staged) Dest() string { return s.dest }

// Path is where the staged content lives until Commit: verify or digest
// it here, before taking the lock.
func (s *Staged) Path() string { return s.temp }

// StageFile writes a new regular file for dest through fill, flushes it
// to disk, and sets mode's permission bits exactly. Missing parent
// directories are created (and removed again on Discard). No lock is held:
// fill may download or compute for as long as it needs.
func StageFile(ctx context.Context, dest string, mode fs.FileMode, fill func(io.Writer) error) (*Staged, error) {
	s, err := newStaged(ctx, dest, false)
	if err != nil {
		return nil, err
	}
	f, err := createStagingFile(s.stagingPath)
	if err != nil {
		s.discardParents()
		return nil, fmt.Errorf("publish: stage %s: %w", dest, err)
	}
	s.temp = f.Name()
	if err := s.own(); err != nil {
		_ = f.Close()
		return nil, s.abandon(err)
	}
	if err := fillFile(ctx, f, mode, fill); err != nil {
		return nil, s.abandon(err)
	}
	return s, nil
}

// StageTree creates a new directory for dest, lets fill populate it,
// and sets the root's permission bits to mode (0 is 0755). fill receives
// the staging root and must write only beneath it.
func StageTree(ctx context.Context, dest string, mode fs.FileMode, fill func(ctx context.Context, root string) error) (*Staged, error) {
	s, err := newStaged(ctx, dest, true)
	if err != nil {
		return nil, err
	}
	temp, err := createStagingDir(s.stagingPath)
	if err != nil {
		s.discardParents()
		return nil, fmt.Errorf("publish: stage %s: %w", dest, err)
	}
	s.temp = temp
	if err := s.own(); err != nil {
		return nil, s.abandon(err)
	}
	if err := fill(ctx, temp); err != nil {
		return nil, s.abandon(fmt.Errorf("fill staged tree: %w", err))
	}
	if err := ctx.Err(); err != nil {
		return nil, s.abandon(err)
	}
	if mode == 0 {
		mode = defaultTreeMode
	}
	if err := os.Chmod(temp, mode.Perm()); err != nil {
		return nil, s.abandon(fmt.Errorf("chmod staged tree: %w", err))
	}
	return s, nil
}

// Discard removes the staged content and any parents staging created.
func (s *Staged) Discard() error {
	if s.spent {
		return s.lease.release()
	}
	s.spent = true
	err := os.RemoveAll(s.temp)
	s.discardParents()
	err = errors.Join(err, s.lease.release())
	if err != nil {
		return fmt.Errorf("publish: discard %s: %w", s.temp, err)
	}
	return nil
}

func newStaged(ctx context.Context, dest string, tree bool) (*Staged, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("publish: stage %s: %w", dest, err)
	}
	abs, err := filepath.Abs(dest)
	if err != nil {
		return nil, fmt.Errorf("publish: stage %s: %w", dest, err)
	}
	s := &Staged{dest: abs, tree: tree, apart: StagesApart(abs)}
	if s.apart {
		reapOrphanedStagesOnce(stagingRoot())
		return s, nil // parents are made at commit, under no ancestor's swap
	}
	if s.created, err = makeParents(filepath.Dir(abs)); err != nil {
		return nil, fmt.Errorf("publish: stage %s: %w", dest, err)
	}
	return s, nil
}

// stagingPath is a fresh staging path for s: in the staging root when s is
// prepared apart, else beside dest.
func (s *Staged) stagingPath() string {
	name := stagingName(s.dest)
	if s.apart {
		root := stagingRoot()
		_ = os.MkdirAll(root, stagingRootMode) // recreate a root deleted since it was resolved
		return filepath.Join(root, filepath.Base(name))
	}
	return name
}

// abandon discards s and returns cause wrapped with the destination.
func (s *Staged) abandon(cause error) error {
	_ = s.Discard()
	return fmt.Errorf("publish: stage %s: %w", s.dest, cause)
}

// discardParents removes, innermost first, the parent directories staging
// created, leaving any that something else has since filled.
func (s *Staged) discardParents() {
	for _, v := range slices.Backward(s.created) {
		if os.Remove(v) != nil {
			break
		}
	}
	s.created = nil
}

// fillFile runs fill into f, then flushes, sets mode, and closes it.
func fillFile(ctx context.Context, f *os.File, mode fs.FileMode, fill func(io.Writer) error) error {
	err := fill(f)
	if err == nil {
		err = ctx.Err()
	}
	if err == nil {
		err = f.Sync()
	}
	if err == nil {
		err = f.Chmod(mode.Perm())
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("fill staged file %s: %w", f.Name(), err)
	}
	return nil
}

// makeParents creates dir and any missing ancestors, returning the ones it
// created, outermost first.
func makeParents(dir string) ([]string, error) {
	var missing []string
	for at := dir; filepath.Dir(at) != at; at = filepath.Dir(at) {
		if _, err := os.Lstat(at); !errors.Is(err, fs.ErrNotExist) {
			break
		}
		missing = append([]string{at}, missing...)
	}
	if err := os.MkdirAll(dir, parentDirMode); err != nil {
		return nil, fmt.Errorf("create parent directories %s: %w", dir, err)
	}
	return missing, nil
}

// stagingName is a fresh staging path beside dest, owned by dest.
func stagingName(dest string) string { return stagename.New(dest) }

// Leftovers lists the staging entries beside dest that belong to it: a
// tree an interrupted commit staged but never published, or the original
// a commit swapped out and had not yet deleted (Commit says which crash
// leaves which). dest itself is always one whole tree. Presence proves
// nothing about content: a caller identifies an original by the digest it
// planned against, and reaps nothing while a writer of dest may still be
// staging.
func Leftovers(dest string) ([]string, error) {
	abs, err := filepath.Abs(dest)
	if err != nil {
		return nil, fmt.Errorf("publish: leftovers of %s: %w", dest, err)
	}
	dir := filepath.Dir(abs)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("publish: leftovers of %s: %w", abs, err)
	}
	owner := stagename.Owner(abs)
	var found []string
	for _, entry := range entries {
		if name := entry.Name(); IsStaging(name) && strings.HasPrefix(name, owner) {
			found = append(found, filepath.Join(dir, name))
		}
	}
	return found, nil
}

func createStagingFile(next func() string) (*os.File, error) {
	for range stagingAttempts {
		f, err := os.OpenFile(next(), os.O_RDWR|os.O_CREATE|os.O_EXCL, stagingFileMode)
		if err == nil {
			return f, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, fmt.Errorf("create staging file: %w", err)
		}
	}
	return nil, fmt.Errorf("no unused staging name after %d attempts", stagingAttempts)
}

func createStagingDir(next func() string) (string, error) {
	for range stagingAttempts {
		name := next()
		err := os.Mkdir(name, stagingDirMode)
		if err == nil {
			return name, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", fmt.Errorf("create staging directory: %w", err)
		}
	}
	return "", fmt.Errorf("no unused staging name after %d attempts", stagingAttempts)
}
