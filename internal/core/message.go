package core

// Visibility selects whether a message is ordinary or verbose user detail.
// Zero is VisibilityNormal.
type Visibility uint8

const (
	// VisibilityNormal messages always project at VerbosityNormal (C11:
	// prefixed consistently with VisibilityVerbose — the two enum members
	// previously disagreed on their own naming convention).
	VisibilityNormal Visibility = iota
	// VisibilityVerbose messages project only when Config.Verbosity is VerbosityVerbose.
	VisibilityVerbose
)

// MessageSnapshot is one logical user-facing message in the canonical model.
type MessageSnapshot struct {
	ID         string
	Text       string
	Visibility Visibility
}
