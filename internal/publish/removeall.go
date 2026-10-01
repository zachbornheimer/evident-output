package publish

import (
	"io/fs"
	"os"
	"path/filepath"
)

// removeAll is os.RemoveAll that also clears read-only directories, such as
// a Go module cache entry (0555 all the way down). It retries only after a
// plain RemoveAll fails, and never follows symlinks.
func removeAll(path string) error {
	err := os.RemoveAll(path)
	if err == nil {
		return nil
	}
	_ = filepath.WalkDir(path, func(p string, d fs.DirEntry, walkErr error) error {
		if d != nil && d.IsDir() {
			_ = os.Chmod(p, 0o700)
		}
		return nil
	})
	return os.RemoveAll(path)
}
