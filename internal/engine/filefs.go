package engine

import "github.com/zachbornheimer/evident-output/internal/fs"

// Every filesystem call evo.File makes lives behind internal/fs, so file.go
// and file_reconcile.go hold only reconciliation logic. The root package
// keeps compiling through these aliases until it speaks internal/fs directly.
type (
	// FileFS is the facade evo.File performs every filesystem inspection and
	// mutation through, instead of the os package directly (facade rule): the
	// real filesystem backs every production Output; a test injects a fake to
	// script a filesystem outcome (e.g. a chmod failure) deterministically,
	// without depending on a real disk's permission behavior.
	FileFS = fs.FS
	// osFileFS is FileFS's real implementation, backing every production Output.
	osFileFS = fs.Disk
)

// unmanagedCreateMode is the permission a new file whose mode is unmanaged
// is created with, before the umask.
const unmanagedCreateMode = fs.UnmanagedCreateMode

// createOrdinary creates a new file whose mode is unmanaged under the process
// umask on the real filesystem, and through WriteAtomic on any other FileFS.
var createOrdinary = fs.CreateOrdinary

// getwd is the facade File's relative-path resolution reads the process's
// current working directory through (facade rule).
var getwd = fs.Getwd
