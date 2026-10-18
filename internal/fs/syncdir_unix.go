//go:build !windows

package fs

import (
	"fmt"
	"os"
)

// SyncDir flushes dir's entries to stable storage, so a rename or create
// inside it survives a crash. Writing and fsyncing a file makes its contents
// durable; only fsyncing the directory makes its name durable.
func SyncDir(dir string) error {
	handle, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open directory %q: %w", dir, err)
	}
	if err := handle.Sync(); err != nil {
		_ = handle.Close()
		return fmt.Errorf("fsync directory %q: %w", dir, err)
	}
	if err := handle.Close(); err != nil {
		return fmt.Errorf("close directory %q: %w", dir, err)
	}
	return nil
}
