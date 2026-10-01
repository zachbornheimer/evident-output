//go:build linux

package engine

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// cloneEntry has no whole-subtree clone on linux; copyEntry walks the
// tree and reflinks each file.
func cloneEntry(string, string) error { return errSubtreeCloneUnsupported }

var errSubtreeCloneUnsupported = errors.New("subtree clone unsupported")

// reflinkFile shares in's extents with out (FICLONE) where the filesystem
// supports it (btrfs, xfs).
func reflinkFile(in, out *os.File) error {
	return unix.IoctlFileClone(int(out.Fd()), int(in.Fd()))
}
