package checksum

import "errors"

// Errors the engine reports for a path of the wrong kind or a bad option.
// Callers map them onto their own public vocabulary.
var (
	// ErrSymlink is a File digest requested of a symlink: the engine never
	// follows one implicitly.
	ErrSymlink = errors.New("checksum: path is a symlink")
	// ErrNotRegular is a File digest requested of a directory, device,
	// FIFO, or socket.
	ErrNotRegular = errors.New("checksum: path is not a regular file")
	// ErrNotDirectory is a Tree digest requested of a non-directory.
	ErrNotDirectory = errors.New("checksum: path is not a directory")
	// ErrInvalidExclude is an exclusion pattern that does not compile.
	ErrInvalidExclude = errors.New("checksum: invalid exclusion pattern")
)
