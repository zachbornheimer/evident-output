package evo

import "github.com/zachbornheimer/evident-output/internal/engine"

// Resource names one unit of shared state an operation coordinates on. It
// is sealed: only FSResource and LogicalResource make one, and no public
// API locks or unlocks it — File and Basis claim their own paths, and an
// opaque operation claims at most one Resource.
//
// Read claims share; any overlapping pair that includes a write waits.
// Code already holding a resource that asks for a second one fails with
// ErrNestedResourceAcquisition instead of risking deadlock.
type Resource = engine.Resource

// FSResource names the filesystem path path. A relative path resolves
// against the Run's workspace directory, like FileSpec.Path, and symlinked
// ancestors resolve to their targets so an alias shares identity with the
// real path. A claim on a directory overlaps every claim beneath it, so a
// coarse claim on a repository root excludes a File write inside it.
// Construction performs no I/O.
func FSResource(path string) Resource { return engine.FSResource(path) }

// LogicalResource names non-filesystem shared state (a package manager, a
// remote, a database) where no truthful filesystem path exists. Logical
// resources overlap only when their names match exactly after surrounding
// whitespace is trimmed; they are not hierarchical.
func LogicalResource(name string) Resource { return engine.LogicalResource(name) }

// Resource misuse errors.
var (
	ErrNestedResourceAcquisition = engine.ErrNestedResourceAcquisition
	ErrInvalidResource           = engine.ErrInvalidResource
)
