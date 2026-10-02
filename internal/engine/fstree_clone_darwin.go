//go:build darwin

package engine

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// cloneFlags clone without following a symlink and without copying the
// source's owner, as a copy made by this process would.
const cloneFlags = unix.CLONE_NOFOLLOW | unix.CLONE_NOOWNERCOPY

// cloneEntry clones the entry at from, a whole subtree for a directory,
// copy-on-write (clonefile). It fails where the filesystem cannot clone,
// and the caller copies instead.
func cloneEntry(from, to string) error { return unix.Clonefile(from, to, cloneFlags) }

// reflinkFile has no per-descriptor clone here: cloneEntry already cloned
// whatever the filesystem could.
func reflinkFile(*os.File, *os.File) error { return errReflinkUnsupported }

var errReflinkUnsupported = errors.New("reflink unsupported")
