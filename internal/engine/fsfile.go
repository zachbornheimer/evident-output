package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"

	"github.com/zachbornheimer/evident-output/internal/publish"
)

// defaultFileMode is the permission a new File gets when Mode is 0.
const defaultFileMode fs.FileMode = 0o644

// FileSource is what a File's Content produces: the bytes to publish, and
// the test for whether a path already holds them. Bytes and Download
// each supply one, so File's write path knows neither.
type FileSource interface {
	// Fill writes the desired bytes. It runs outside any lock and may
	// download or compute for as long as it needs.
	Fill(ctx context.Context, w io.Writer) error
	// Holds reports whether the regular file at path already holds the
	// desired bytes, without producing them again.
	Holds(ctx context.Context, path string) (bool, error)
}

// BytesSource is the FileSource for literal bytes.
func BytesSource(data []byte) FileSource { return literalSource{data: data} }

type literalSource struct{ data []byte }

func (s literalSource) Fill(_ context.Context, w io.Writer) error {
	_, err := w.Write(s.data)
	return err
}

func (s literalSource) Holds(ctx context.Context, path string) (bool, error) {
	out := checksumScope(ctx)
	data, err := out.fileFSOrDefault().ReadFile(path)
	if err != nil {
		return false, err
	}
	return bytes.Equal(data, s.data), nil
}

// FileRead is File.Read: the observed bytes of the regular file at path.
func FileRead(ctx context.Context, path string) ([]byte, error) {
	if path == "" {
		return nil, ErrPathMissing
	}
	out := checksumScope(ctx)
	abs := out.checksumPath(path)
	if _, _, err := inspectFilePath(out.fileFSOrDefault(), abs); err != nil {
		return nil, err
	}
	data, err := out.fileFSOrDefault().ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("evo: File %q Read: %w", path, err)
	}
	return data, nil
}

// FileEqual is File.Equal: whether both files hold the same bytes, by the
// shared checksum engine. Mode is ignored.
func FileEqual(ctx context.Context, a, b string) (bool, error) {
	if a == "" || b == "" {
		return false, ErrPathMissing
	}
	out := checksumScope(ctx)
	left, err := out.fileDigest(ctx, out.checksumPath(a))
	if err != nil {
		return false, fmt.Errorf("evo: File %q Equal: %w", a, err)
	}
	right, err := out.fileDigest(ctx, out.checksumPath(b))
	if err != nil {
		return false, fmt.Errorf("evo: File %q Equal: %w", b, err)
	}
	return left == right, nil
}

// FileVerify is File.Verify: nil when path holds src (a nil src checks
// only that a regular file exists) and mode (0 is unchecked), else
// ErrVerifyMismatch. Observing never needs a Task.
func FileVerify(ctx context.Context, path string, src FileSource, mode fs.FileMode) error {
	if path == "" {
		return ErrPathMissing
	}
	out := checksumScope(ctx)
	abs := out.checksumPath(path)
	ok, err := fileSatisfied(ctx, out.fileFSOrDefault(), abs, src, mode)
	if err != nil {
		return fmt.Errorf("evo: File %q Verify: %w", path, err)
	}
	if !ok {
		return fmt.Errorf("evo: File %q Verify: %w", path, ErrVerifyMismatch)
	}
	return nil
}

// fileSatisfied reports whether abs is a regular file holding src with
// mode. A missing path is not satisfied; a symlink or non-regular path is
// an error.
func fileSatisfied(ctx context.Context, fsys FileFS, abs string, src FileSource, mode fs.FileMode) (bool, error) {
	info, exists, err := inspectFilePath(fsys, abs)
	if err != nil || !exists {
		return false, err
	}
	if mode != 0 && info.Mode().Perm() != mode.Perm() {
		return false, nil
	}
	if src == nil {
		return true, nil
	}
	holds, err := src.Holds(ctx, abs)
	if errors.Is(err, fs.ErrNotExist) {
		// Removed between the inspect and the read: absent, not satisfied.
		return false, nil
	}
	return holds, err
}

// FileWrite is File.Write. Content is produced outside any lock into a
// staging file beside path, then published by atomic rename under the
// destination lock, which revalidates first and verifies after. Content
// and mode already in place is a no-op that keeps the inode.
func FileWrite(ctx context.Context, path string, src FileSource, mode fs.FileMode) error {
	label := fmt.Sprintf("File %q", path)
	task, err := beginOperation(ctx, label+" Write")
	if err != nil {
		return err
	}
	if path == "" {
		return ErrPathMissing
	}
	if src == nil {
		return fmt.Errorf("evo: %s Write: %w", label, ErrContentMissing)
	}
	o := task.out
	abs := o.checksumPath(path)
	fsys := o.fileFS()
	info, exists, err := inspectFilePath(fsys, abs)
	if err != nil {
		return err
	}
	want := mode
	if want == 0 {
		want = defaultFileMode
		if exists {
			want = info.Mode().Perm()
		}
	}
	satisfied, err := fileSatisfied(ctx, fsys, abs, src, want)
	if err != nil {
		return fmt.Errorf("evo: %s Write: %w", label, err)
	}
	if o.DryRun() {
		o.recordFileEffectIf(fileOperation{taskID: task.id, spec: FileSpec{Path: path}, path: abs}, !satisfied)
		return nil
	}
	if satisfied {
		return nil
	}
	guard := publish.Guard{
		Revalidate: func(ctx context.Context, dest string) error {
			ok, err := fileSatisfied(ctx, fsys, dest, src, want)
			if err != nil {
				return err
			}
			if ok {
				return publish.ErrSatisfied
			}
			return nil
		},
		Verify: func(ctx context.Context, dest string) error {
			ok, err := fileSatisfied(ctx, fsys, dest, src, want)
			if err != nil {
				return err
			}
			if !ok {
				return ErrVerifyMismatch
			}
			return nil
		},
	}
	fill := func(w io.Writer) error { return src.Fill(ctx, w) }
	if exists {
		// Content already in place and only the mode differs: restage the
		// bytes on disk instead of producing them again (a Download must
		// not refetch to change a mode).
		if held, herr := src.Holds(ctx, abs); herr == nil && held {
			fill = func(w io.Writer) error {
				data, err := fsys.ReadFile(abs)
				if err != nil {
					return err
				}
				_, err = w.Write(data)
				return err
			}
		}
	}
	staged, err := publish.StageFile(ctx, abs, want, fill)
	if err != nil {
		return fmt.Errorf("evo: %s Write: %w", label, err)
	}
	if err := staged.Commit(ctx, guard); err != nil {
		return fmt.Errorf("evo: %s Write: %w", label, err)
	}
	op := fileOperation{taskID: task.id, spec: FileSpec{Path: path}, path: abs}
	o.recordFileEffectIf(op, true)
	return nil
}
