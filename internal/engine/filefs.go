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
	// ordinaryCreator is a FileFS that can create a file under the process
	// umask itself.
	ordinaryCreator = fs.OrdinaryCreator
)

// unmanagedCreateMode is the permission a new file whose mode is unmanaged
// is created with, before the umask.
const unmanagedCreateMode = fs.UnmanagedCreateMode

// getwd is the facade File's relative-path resolution reads the process's
// current working directory through (facade rule).
var getwd = fs.Getwd
