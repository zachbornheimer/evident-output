package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/zachbornheimer/evident-output/internal/checksum"
)

// ErrCloneUnsupportedEntry is a source entry a clone cannot reproduce: a
// FIFO, socket, or device.
var ErrCloneUnsupportedEntry = errors.New("evo: Clone cannot copy a special file")

// TreeClone is the TreeSource for Clone: a copy of the tree at from. Each
// entry is cloned copy-on-write where the platform offers it (darwin
// clonefile, linux FICLONE) and copied byte for byte otherwise, never
// hard-linked, so the copy is independent of its source. The copy must
// digest to the source's digest, or Fill fails. A writable copy is made
// owner-writable after cloning (see makeOwnerWritable); a mode change
// shares no less data, and the digest, which counts only the exec bit,
// is unchanged.
func TreeClone(from string, writable bool) TreeSource {
	return cloneSource{from: from, writable: writable}
}

type cloneSource struct {
	from     string
	writable bool
}

// Modes a writable clone gives its entries. A file keeps its execute bits
// on top of ownerWritableFile, so executables stay executable.
const (
	ownerWritableDir  fs.FileMode = 0o755
	ownerWritableFile fs.FileMode = 0o644
	executeBits       fs.FileMode = 0o111
)

// Fill implements TreeSource.
func (c cloneSource) Fill(ctx context.Context, root string) error {
	if c.from == "" {
		return fmt.Errorf("evo: Clone source: %w", ErrPathMissing)
	}
	out := checksumScope(ctx)
	src, err := out.resolveTreeRoot(out.checksumPath(c.from))
	if err != nil {
		return fmt.Errorf("evo: Clone %s: %w", c.from, err)
	}
	if err := out.requireTreeDirectory(src); err != nil {
		return fmt.Errorf("evo: Clone %s: %w", c.from, err)
	}
	want, err := out.treeDigest(ctx, src, checksum.Exclusion{})
	if err != nil {
		return fmt.Errorf("evo: Clone %s: digest source: %w", c.from, err)
	}
	if err := cloneChildren(ctx, src, root); err != nil {
		return fmt.Errorf("evo: Clone %s: %w", c.from, err)
	}
	if c.writable {
		if err := makeOwnerWritable(root); err != nil {
			return fmt.Errorf("evo: Clone %s: make writable: %w", c.from, err)
		}
	}
	have, err := out.treeDigest(ctx, root, checksum.Exclusion{})
	if err != nil {
		return fmt.Errorf("evo: Clone %s: digest copy: %w", c.from, err)
	}
	if have != want {
		return fmt.Errorf("evo: Clone %s: %w: copy digests to %s, source to %s", c.from, ErrVerifyMismatch, have, want)
	}
	return nil
}

// makeOwnerWritable sets every directory beneath root to ownerWritableDir
// and every regular file to ownerWritableFile plus its own execute bits.
// A directory is changed before it is read, so a read-only source's
// subtree opens. Symlinks are left alone; root is the staging directory.
func makeOwnerWritable(root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || path == root {
			return err
		}
		switch {
		case entry.IsDir():
			return os.Chmod(path, ownerWritableDir)
		case entry.Type().IsRegular():
			info, err := entry.Info()
			if err != nil {
				return err
			}
			return os.Chmod(path, ownerWritableFile|info.Mode().Perm()&executeBits)
		}
		return nil
	})
}

// cloneChildren copies every entry of the directory src into the existing
// directory dst.
func cloneChildren(ctx context.Context, src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("list %s: %w", src, err)
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		from, to := filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())
		if cloneEntry(from, to) == nil {
			continue
		}
		// A failed clone may have left part of to behind; to is inside
		// the private staging root, so clearing it touches nothing else.
		_ = os.RemoveAll(to)
		if err := copyEntry(ctx, from, to); err != nil {
			return err
		}
	}
	return nil
}

// copyEntry reproduces one entry without platform cloning of the whole
// subtree: directories recursively, files by reflink or bytes, symlinks
// by their link text.
func copyEntry(ctx context.Context, from, to string) error {
	info, err := os.Lstat(from)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", from, err)
	}
	switch mode := info.Mode(); {
	case mode.IsDir():
		if err := os.Mkdir(to, mode.Perm()); err != nil {
			return fmt.Errorf("create %s: %w", to, err)
		}
		if err := cloneChildren(ctx, from, to); err != nil {
			return err
		}
		return os.Chmod(to, mode.Perm())
	case mode.IsRegular():
		return copyFile(from, to, mode.Perm())
	case mode&fs.ModeSymlink != 0:
		target, err := os.Readlink(from)
		if err != nil {
			return fmt.Errorf("read link %s: %w", from, err)
		}
		return os.Symlink(target, to)
	default:
		return fmt.Errorf("%w: %s is %v", ErrCloneUnsupportedEntry, from, mode.Type())
	}
}

// copyFile writes a new regular file at to with from's bytes and perm.
func copyFile(from, to string, perm fs.FileMode) (err error) {
	in, err := os.Open(from)
	if err != nil {
		return fmt.Errorf("open %s: %w", from, err)
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return fmt.Errorf("create %s: %w", to, err)
	}
	defer func() { err = errors.Join(err, out.Close()) }()
	if reflinkFile(in, out) != nil {
		if _, err := io.Copy(out, in); err != nil {
			return fmt.Errorf("copy %s: %w", from, err)
		}
	}
	return out.Chmod(perm)
}
