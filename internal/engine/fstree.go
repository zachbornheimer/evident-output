package engine

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/zachbornheimer/evident-output/internal/checksum"
	"github.com/zachbornheimer/evident-output/internal/publish"
)

// TreeSource is what a Tree's Content produces: the whole desired tree,
// written into an empty staging directory. Extract supplies one, so
// Tree's write path knows no archive format.
type TreeSource interface {
	// Fill populates root, an empty directory, with the desired tree. It
	// runs outside any lock and may read or compute for as long as it
	// needs; it must write only beneath root and must honor ctx.
	Fill(ctx context.Context, root string) error
}

// ResolvePath resolves path against the workspace of the Run ctx belongs
// to, or the process working directory outside a Task. TreeSource
// implementations use it for any path they read.
func ResolvePath(ctx context.Context, path string) string {
	return checksumScope(ctx).checksumPath(path)
}

// TreeRead is Tree.Read: the regular files under the directory at path,
// as absolute paths in directory-walk order. Symlinks and special files are
// not listed and nothing is followed below the root; no file is opened.
func TreeRead(ctx context.Context, path string) ([]string, error) {
	if path == "" {
		return nil, ErrPathMissing
	}
	out := checksumScope(ctx)
	abs := out.checksumPath(path)
	root, err := out.resolveTreeRoot(abs)
	if err != nil {
		return nil, fmt.Errorf("evo: Tree %q Read: %w", path, err)
	}
	src := checksumSource{fsys: out.fileFSOrDefault()}
	info, err := src.Lstat(root)
	if err != nil {
		return nil, fmt.Errorf("evo: Tree %q Read: %w", path, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("evo: Tree %q Read: %w", path, ErrTreePathTypeMismatch)
	}
	var files []string
	if err := listRegular(ctx, src, root, abs, &files); err != nil {
		return nil, fmt.Errorf("evo: Tree %q Read: %w", path, err)
	}
	return files, nil
}

// listRegular appends the regular files under dir (read at readAt) to
// files, naming them beneath shownAs, in sorted order, skipping staging
// entries.
func listRegular(ctx context.Context, src checksumSource, readAt, shownAs string, files *[]string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	entries, err := src.ReadDir(readAt)
	if err != nil {
		return err
	}
	slices.SortFunc(entries, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	for _, entry := range entries {
		name := entry.Name()
		if publish.IsStaging(name) {
			continue
		}
		switch {
		case entry.Type().IsRegular():
			*files = append(*files, filepath.Join(shownAs, name))
		case entry.IsDir():
			if err := listRegular(ctx, src, filepath.Join(readAt, name), filepath.Join(shownAs, name), files); err != nil {
				return err
			}
		}
	}
	return nil
}

// TreeEqual is Tree.Equal: whether the trees at a and b have one Merkle
// digest under the exclusion patterns, the same identity Tree.Checksum
// reports.
func TreeEqual(ctx context.Context, a, b string, exclude []string) (bool, error) {
	if a == "" || b == "" {
		return false, ErrPathMissing
	}
	exclusion, err := checksum.CompileExclusion(exclude...)
	if err != nil {
		return false, fmt.Errorf("evo: Tree %q Equal: %w", a, err)
	}
	out := checksumScope(ctx)
	left, err := out.treeDigest(ctx, out.checksumPath(a), exclusion)
	if err != nil {
		return false, fmt.Errorf("evo: Tree %q Equal: %w", a, err)
	}
	right, err := out.treeDigest(ctx, out.checksumPath(b), exclusion)
	if err != nil {
		return false, fmt.Errorf("evo: Tree %q Equal: %w", b, err)
	}
	return left == right, nil
}

// TreeWrite is Tree.Write: it prepares src's tree beside path outside any
// lock, then publishes it by atomic rename under the destination lock,
// replacing whatever tree was there wholesale, and verifies the result
// before releasing the lock. A tree that already holds the desired content
// is left alone. A failed or cancelled Write leaves the previous tree and
// no staging behind.
func TreeWrite(ctx context.Context, path string, src TreeSource) error {
	return publishTree(ctx, treePublication{
		verb: "Write",
		path: path,
		src:  src,
		precheck: func(o *Output, abs string) error {
			return o.requireTreeOrAbsent(abs)
		},
		admit: func(_ context.Context, o *Output, dest string, _ checksum.Digest) error {
			return o.requireTreeOrAbsent(dest)
		},
		skipSatisfied: true,
	})
}

// ErrTreeChanged is a Tree.Replace whose destination no longer holds the
// expected tree; the destination is left exactly as found.
var ErrTreeChanged = errors.New("evo: tree changed since it was observed")

// TreeReplace is Tree.Replace: TreeWrite that commits only if, inside the
// destination's critical section, the tree at path still digests to
// expected (a Tree.Checksum result). Otherwise it is ErrTreeChanged and
// the destination is untouched. A missing destination has changed too.
func TreeReplace(ctx context.Context, path string, src TreeSource, expected string) error {
	return publishTree(ctx, treePublication{
		verb: "Replace",
		path: path,
		src:  src,
		precheck: func(o *Output, abs string) error {
			return o.requireTreeOrAbsent(abs)
		},
		admit: func(ctx context.Context, o *Output, dest string, want checksum.Digest) error {
			have, err := o.treeDigest(ctx, dest, checksum.Exclusion{})
			switch {
			case errors.Is(err, fs.ErrNotExist):
				return fmt.Errorf("%w: %s is gone", ErrTreeChanged, dest)
			case err != nil:
				return err
			case have.String() != expected:
				return fmt.Errorf("%w: %s digests to %s, expected %s", ErrTreeChanged, dest, have, expected)
			case have == want:
				return publish.ErrSatisfied
			}
			return nil
		},
	})
}

// treePublication is one Tree publication: what is staged where, and what
// the destination must still satisfy when the commit runs.
type treePublication struct {
	verb string
	path string
	src  TreeSource
	// precheck refuses a destination before any staging work.
	precheck func(o *Output, abs string) error
	// admit re-proves, inside the destination's critical section, that
	// the destination may be replaced by the staged tree digesting to want.
	admit func(ctx context.Context, o *Output, dest string, want checksum.Digest) error
	// skipSatisfied discards the staged tree before the critical section
	// when the destination already holds it.
	skipSatisfied bool
}

// publishTree prepares p.src's tree beside the destination outside any
// lock, then commits it under the destination's coordination: admit,
// atomic swap, verify against the staged digest, release.
func publishTree(ctx context.Context, p treePublication) error {
	label := fmt.Sprintf("Tree %q %s", p.path, p.verb)
	task, err := beginOperation(ctx, label)
	if err != nil {
		return err
	}
	if p.path == "" {
		return ErrPathMissing
	}
	if p.src == nil {
		return fmt.Errorf("evo: %s: %w", label, ErrContentMissing)
	}
	out := task.out
	abs := out.checksumPath(p.path)
	if err := p.precheck(out, abs); err != nil {
		return fmt.Errorf("evo: %s: %w", label, err)
	}
	if out.DryRun() {
		return nil
	}
	staged, err := publish.StageTree(ctx, abs, 0, func(ctx context.Context, root string) error { return p.src.Fill(ctx, root) })
	if err != nil {
		return fmt.Errorf("evo: %s: %w", label, err)
	}
	want, err := out.treeDigest(ctx, staged.Path(), checksum.Exclusion{})
	if err != nil {
		_ = staged.Discard()
		return fmt.Errorf("evo: %s: digest prepared tree: %w", label, err)
	}
	if p.skipSatisfied {
		if have, err := out.treeDigest(ctx, abs, checksum.Exclusion{}); err == nil && have == want {
			return staged.Discard()
		}
	}
	guard := publish.Guard{
		Revalidate: func(ctx context.Context, dest string) error { return p.admit(ctx, out, dest, want) },
		Verify: func(ctx context.Context, dest string) error {
			return out.requireTreeDigest(ctx, dest, want)
		},
	}
	err = staged.Commit(ctx, guard)
	switch {
	case errors.Is(err, publish.ErrStagedGone):
		// An ancestor's commit carried the stage away: the tree this
		// publication was planned against is gone.
		return fmt.Errorf("evo: %s: %w: %w", label, ErrTreeChanged, err)
	case err != nil:
		return fmt.Errorf("evo: %s: %w", label, err)
	}
	return nil
}

// TreeVerify is Tree.Verify: nil when the tree at path holds exactly src's
// tree, else ErrVerifyMismatch. src is prepared beside path and discarded.
func TreeVerify(ctx context.Context, path string, src TreeSource) error {
	label := fmt.Sprintf("Tree %q Verify", path)
	task, err := beginOperation(ctx, label)
	if err != nil {
		return err
	}
	if path == "" {
		return ErrPathMissing
	}
	if src == nil {
		return fmt.Errorf("evo: %s: %w", label, ErrContentMissing)
	}
	out := task.out
	abs := out.checksumPath(path)
	if err := out.requireTreeDirectory(abs); err != nil {
		return fmt.Errorf("evo: %s: %w: %w", label, ErrVerifyMismatch, err)
	}
	staged, err := publish.StageTree(ctx, abs, 0, func(ctx context.Context, root string) error { return src.Fill(ctx, root) })
	if err != nil {
		return fmt.Errorf("evo: %s: %w", label, err)
	}
	defer func() { _ = staged.Discard() }()
	want, err := out.treeDigest(ctx, staged.Path(), checksum.Exclusion{})
	if err != nil {
		return fmt.Errorf("evo: %s: digest prepared tree: %w", label, err)
	}
	if err := out.requireTreeDigest(ctx, abs, want); err != nil {
		return fmt.Errorf("evo: %s: %w", label, err)
	}
	return nil
}

// requireTreeDigest is nil when the directory at path digests to want, and
// ErrVerifyMismatch otherwise.
func (o *Output) requireTreeDigest(ctx context.Context, path string, want checksum.Digest) error {
	have, err := o.treeDigest(ctx, path, checksum.Exclusion{})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrVerifyMismatch, err)
	}
	if have != want {
		return fmt.Errorf("%w: %s differs from the declared tree", ErrVerifyMismatch, path)
	}
	return nil
}

// requireTreeOrAbsent accepts a path that is absent or a directory, and
// refuses a symlink or any other entry, so a Write never publishes through
// or over something that is not a tree.
func (o *Output) requireTreeOrAbsent(path string) error {
	err := o.requireTreeDirectory(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// requireTreeDirectory is nil only for a real directory at path (not a
// symlink to one).
func (o *Output) requireTreeDirectory(path string) error {
	info, err := o.fileFSOrDefault().Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: %s is not a directory (%s)", ErrTreePathTypeMismatch, path, info.Mode().Type())
	}
	return nil
}
