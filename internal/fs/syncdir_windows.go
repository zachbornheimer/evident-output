//go:build windows

package fs

// SyncDir is a no-op on Windows: a directory cannot be opened for fsync, and
// NTFS journals the rename itself.
func SyncDir(string) error { return nil }
