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
	label := fmt.Sprintf("Tree %q Write", path)
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
	if err := out.requireTreeOrAbsent(abs); err != nil {
		return fmt.Errorf("evo: %s: %w", label, err)
	}
	if out.DryRun() {
		return nil
	}
	staged, err := publish.StageTree(ctx, abs, 0, func(ctx context.Context, root string) error { return src.Fill(ctx, root) })
	if err != nil {
		return fmt.Errorf("evo: %s: %w", label, err)
	}
	want, err := out.treeDigest(ctx, staged.Path(), checksum.Exclusion{})
	if err != nil {
		_ = staged.Discard()
		return fmt.Errorf("evo: %s: digest prepared tree: %w", label, err)
	}
	if have, err := out.treeDigest(ctx, abs, checksum.Exclusion{}); err == nil && have == want {
		return staged.Discard()
	}
	guard := publish.Guard{
		Revalidate: func(_ context.Context, dest string) error { return out.requireTreeOrAbsent(dest) },
		Verify: func(ctx context.Context, dest string) error {
			return out.requireTreeDigest(ctx, dest, want)
		},
	}
	if err := staged.Commit(ctx, guard); err != nil {
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
