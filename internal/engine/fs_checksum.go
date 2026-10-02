package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"

	"github.com/zachbornheimer/evident-output/internal/checksum"
)

// FileChecksum is File.Checksum: the lowercase hex SHA-256 of the regular
// file at path, read through the ctx's Output FileFS (or the real
// filesystem outside a Task). It is the same leaf digest TreeChecksum uses.
func FileChecksum(ctx context.Context, path string) (string, error) {
	if path == "" {
		return "", ErrPathMissing
	}
	out := checksumScope(ctx)
	digest, err := out.fileDigest(ctx, out.checksumPath(path))
	if err != nil {
		return "", fmt.Errorf("evo: File %q Checksum: %w", path, err)
	}
	return digest.String(), nil
}

// TreeChecksum is Tree.Checksum: the Merkle digest of the directory at
// path, leaving out every entry an exclude pattern matches.
func TreeChecksum(ctx context.Context, path string, exclude []string) (string, error) {
	if path == "" {
		return "", ErrPathMissing
	}
	exclusion, err := checksum.CompileExclusion(exclude...)
	if err != nil {
		return "", fmt.Errorf("evo: Tree %q Checksum: %w", path, err)
	}
	out := checksumScope(ctx)
	digest, err := out.treeDigest(ctx, out.checksumPath(path), exclusion)
	if err != nil {
		return "", fmt.Errorf("evo: Tree %q Checksum: %w", path, err)
	}
	return digest.String(), nil
}

// checksumScope is the Output whose Define callback ctx came from, or nil
// outside any Task. Checksums observe and never mutate, so a closed scope
// still names the Output whose FileFS to read through.
func checksumScope(ctx context.Context) *Output {
	if ctx == nil {
		return nil
	}
	scope, _ := ctx.Value(taskScopeContextKey{}).(*taskScopeHandle)
	if scope == nil {
		return nil
	}
	return scope.out
}

// checksumPath resolves path against o's workspace, or against the process
// working directory outside any Task.
func (o *Output) checksumPath(path string) string {
	if o != nil {
		return o.resolveWorkspacePath(path)
	}
	dir, err := getwd()
	if err != nil {
		dir = "."
	}
	return resolvePathAgainst(dir, path)
}

// checksums is the one checksum engine for o: File.Checksum, Tree.Checksum,
// Equal, and Basis all digest content through it, so a FileFS injected on
// the Output is the only place their bytes come from. A nil o reads the
// real filesystem.
func (o *Output) checksums() checksum.Engine {
	if o == nil {
		return checksum.Engine{}
	}
	return checksum.Engine{Source: checksumSource{fsys: o.fileFS()}}
}

// fileDigest digests the regular file at the absolute path, in File's
// error vocabulary.
func (o *Output) fileDigest(ctx context.Context, path string) (checksum.Digest, error) {
	digest, err := o.checksums().File(ctx, path)
	return digest, fileKindError(err)
}

// treeDigest digests the directory at the absolute path, in Tree's error
// vocabulary. A symlink at path is resolved first; symlinks below it are
// digested as links, never followed.
func (o *Output) treeDigest(ctx context.Context, path string, exclude checksum.Exclusion) (checksum.Digest, error) {
	root, err := o.resolveTreeRoot(path)
	if err != nil {
		return checksum.Digest{}, err
	}
	digest, err := o.checksums().Tree(ctx, root, exclude)
	if errors.Is(err, checksum.ErrNotDirectory) {
		err = fmt.Errorf("%w: %w", ErrTreePathTypeMismatch, err)
	}
	return digest, err
}

// resolveTreeRoot follows a symlink at a Tree's own Path (and only there).
func (o *Output) resolveTreeRoot(path string) (string, error) {
	info, err := checksumSource{fsys: o.fileFSOrDefault()}.Lstat(path)
	if err != nil || info.Mode()&fs.ModeSymlink == 0 {
		return path, nil
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve tree root %s: %w", path, err)
	}
	return resolved, nil
}

func (o *Output) fileFSOrDefault() FileFS {
	if o == nil {
		return osFileFS{}
	}
	return o.fileFS()
}

// fileKindError restates the engine's kind errors as File's.
func fileKindError(err error) error {
	switch {
	case errors.Is(err, checksum.ErrSymlink):
		return fmt.Errorf("%w: %w", ErrFilePathIsSymlink, err)
	case errors.Is(err, checksum.ErrNotRegular):
		return fmt.Errorf("%w: %w", ErrFilePathTypeMismatch, err)
	default:
		return err
	}
}

// checksumSource adapts a FileFS to the checksum engine. Content streams
// through the FileFS's optional Open when it has one, through the engine's
// own non-blocking open for the real filesystem, and through ReadFile
// otherwise. Listings go through an optional ReadDir, else the real
// filesystem.
type checksumSource struct{ fsys FileFS }

// contentOpener is the optional streaming read on a FileFS.
type contentOpener interface {
	Open(path string) (fs.File, error)
}

// directoryReader is the optional listing on a FileFS.
type directoryReader interface {
	ReadDir(path string) ([]fs.DirEntry, error)
}

func (s checksumSource) Lstat(path string) (fs.FileInfo, error) { return s.fsys.Lstat(path) }

func (s checksumSource) Open(path string) (io.ReadCloser, error) {
	if opener, ok := s.fsys.(contentOpener); ok {
		return opener.Open(path)
	}
	if _, real := s.fsys.(osFileFS); real {
		return checksum.OS{}.Open(path)
	}
	data, err := s.fsys.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (s checksumSource) ReadDir(path string) ([]fs.DirEntry, error) {
	if reader, ok := s.fsys.(directoryReader); ok {
		return reader.ReadDir(path)
	}
	return checksum.OS{}.ReadDir(path)
}

func (s checksumSource) Readlink(path string) (string, error) { return checksum.OS{}.Readlink(path) }
