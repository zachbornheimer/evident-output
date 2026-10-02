//go:build !unix

package checksum

import (
	"io"
	"os"
)

// openRegular opens path for reading and refuses anything that is not a
// regular file.
func openRegular(path string) (io.ReadCloser, error) {
	f, err := os.Open(path)
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
