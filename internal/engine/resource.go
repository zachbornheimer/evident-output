package engine

import (
	"context"

	"github.com/zachbornheimer/evident-output/internal/resource"
)

// Resource names one unit of shared state an operation coordinates on
// (ZYS-840). It is sealed: only FSResource and LogicalResource make one.
type Resource = resource.Resource

// FSResource names the filesystem path path. A relative path resolves
// against the Run's workspace directory, the same way FileSpec.Path does,
// and symlinked ancestors resolve to their targets so an alias cannot
// bypass coordination. A claim on a directory overlaps every claim on a
// path beneath it. Construction performs no I/O.
func FSResource(path string) Resource { return resource.FS(path) }

// LogicalResource names non-filesystem shared state (a package manager, a
// remote, a database) where no truthful filesystem path exists. Two
// logical resources overlap only when their names match exactly after
// surrounding whitespace is trimmed; names are not hierarchical.
func LogicalResource(name string) Resource { return resource.Logical(name) }

// Resource access modes stay internal: callers never pick a mode, the
// operation (File, Basis, Effect) implies it.
const (
	resourceRead  = resource.Read
	resourceWrite = resource.Write
)

// processResources coordinates every Output in this process: two Outputs
// touching the same file must exclude each other just as two tasks in one
// Output do.
var processResources = resource.NewRegistry()

// holdResource holds r in mode for fn's duration and releases it however
// fn ends. It is the only way engine code acquires a resource; there is no
// lock/unlock pair to misuse. A ctx that already holds a resource fails
// with ErrNestedResourceAcquisition before any wait.
func (o *Output) holdResource(ctx context.Context, r Resource, mode resource.Mode, fn func(context.Context) error) error {
	req := resource.Request{Resource: r, Workspace: o.workspaceDirLocked(), Mode: mode}
	return processResources.HoldResource(ctx, req, fn)
}

// Resource misuse errors.
var (
	// ErrNestedResourceAcquisition is returned, without waiting, when code
	// already holding a resource (directly, or through any helper it passed
	// its context to) asks for a second one. Holding at most one resource
	// at a time is what makes deadlock impossible, so this is misuse even
	// when the second resource is free.
	ErrNestedResourceAcquisition = resource.ErrNested
	// ErrInvalidResource is returned when a Resource names nothing: an
	// empty path or logical name.
	ErrInvalidResource = resource.ErrInvalid
)
