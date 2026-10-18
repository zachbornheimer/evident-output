//go:build linux

package fs

import "golang.org/x/sys/unix"

// Exchange atomically swaps the entries at a and b (renameat2
// RENAME_EXCHANGE).
func Exchange(a, b string) error {
	return unix.Renameat2(unix.AT_FDCWD, a, unix.AT_FDCWD, b, unix.RENAME_EXCHANGE)
}
