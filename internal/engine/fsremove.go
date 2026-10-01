package engine

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/zachbornheimer/evident-output/internal/publish"
)

// FileRemove is File.Remove: delete the entry at path. An absent path is
// success. A directory is refused. A symlink is unlinked itself, never
// followed, so a dangling link is removed rather than mistaken for absence.
func FileRemove(ctx context.Context, path string) error {
	label := fmt.Sprintf("File %q", path)
	return removeEntry(ctx, path, label, func(info fs.FileInfo, abs string) error {
		if info.IsDir() {
			return fmt.Errorf("%w: %s is a directory", ErrFilePathTypeMismatch, abs)
		}
		if info.Mode()&fs.ModeSymlink == 0 && !info.Mode().IsRegular() {
			return fmt.Errorf("%w: %s", ErrFilePathTypeMismatch, abs)
		}
		return nil
	})
}

// TreeRemove is Tree.Remove: delete the directory at path recursively
// without opening any file. An absent path is success; anything but a
// directory (or a symlink, unlinked itself) is ErrTreePathTypeMismatch.
func TreeRemove(ctx context.Context, path string) error {
	label := fmt.Sprintf("Tree %q", path)
	return removeEntry(ctx, path, label, func(info fs.FileInfo, abs string) error {
		if !info.IsDir() && info.Mode()&fs.ModeSymlink == 0 {
			return fmt.Errorf("%w: %s", ErrTreePathTypeMismatch, abs)
		}
		return nil
	})
}

// removeEntry is the shared Remove flow: inspect, honor DryRun, then delete
// under the destination lock, re-checking the entry's type at commit.
func removeEntry(ctx context.Context, path, label string, accept func(fs.FileInfo, string) error) error {
	task, err := beginOperation(ctx, label+" Remove")
	if err != nil {
		return err
	}
	if path == "" {
		return ErrPathMissing
	}
	o := task.out
	abs := o.checksumPath(path)
	fsys := o.fileFS()
	exists, err := inspectRemovable(fsys, abs, accept)
	if err != nil {
		return err
	}
	op := fileOperation{taskID: task.id, spec: FileSpec{Path: path}, path: abs}
	if o.DryRun() {
		o.recordFileEffectIf(op, exists)
		return nil
	}
	if !exists {
		return nil
	}
	guard := publish.Guard{Revalidate: func(_ context.Context, dest string) error {
		_, err := inspectRemovable(fsys, dest, accept)
		return err
	}}
	if err := publish.Remove(ctx, abs, guard); err != nil {
		return fmt.Errorf("evo: %s Remove: %w", label, err)
	}
	o.recordFileEffectIf(op, true)
	return nil
}

func inspectRemovable(fsys FileFS, abs string, accept func(fs.FileInfo, string) error) (bool, error) {
	info, err := fsys.Lstat(abs)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("evo: inspect %q: %w", abs, err)
	}
	return true, accept(info, abs)
}
