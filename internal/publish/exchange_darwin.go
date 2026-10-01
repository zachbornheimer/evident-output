//go:build darwin

package publish

import "golang.org/x/sys/unix"

// exchange atomically swaps the entries at a and b (renamex_np RENAME_SWAP).
func exchange(a, b string) error {
	return unix.RenameatxNp(unix.AT_FDCWD, a, unix.AT_FDCWD, b, unix.RENAME_SWAP)
}
