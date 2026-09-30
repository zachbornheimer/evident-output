package evo

import "github.com/zachbornheimer/evident-output/internal/core"

// ProgressKind classifies task measurement.
type ProgressKind = core.ProgressKind

// ProgressKind values — which measurement a task's Progress reports.
const (
	Indeterminate = core.Indeterminate
	Determinate   = core.Determinate
	BytesKind     = core.BytesKind
)

// Progress is absolute measurement for a task.
type Progress = core.Progress

// Visibility selects whether a message is ordinary or verbose user detail.
// Zero is VisibilityNormal.
type Visibility = core.Visibility

const (
	// VisibilityNormal messages always project at VerbosityNormal (C11:
	// prefixed consistently with VisibilityVerbose — the two enum members
	// previously disagreed on their own naming convention).
	VisibilityNormal = core.VisibilityNormal
	// VisibilityVerbose messages project only when Config.Verbosity is VerbosityVerbose.
	VisibilityVerbose = core.VisibilityVerbose
)
