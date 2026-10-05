//go:build unix

package checksum

import (
	"io"
	"os"
	"syscall"
)

// openRegular opens path for reading without blocking on a FIFO and
// without following a final symlink, then refuses anything that is not a
// regular file. O_NONBLOCK has no effect on a regular file's reads.
func openRegular(path string) (io.ReadCloser, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		_ = f.Close()
		return nil, openedNonRegular(path, info.Mode())
	}
	return f, nil
}
