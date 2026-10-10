package graph

import "errors"

// ErrDeclaredInCallback is why a declaration was refused: a Task, Group, or
// Sequence was declared from inside a Task's Define callback. Declare it
// from a container builder, or before the run.
var ErrDeclaredInCallback = errors.New("evo: declared from inside a Task callback")

// DeclaredInCallback reports whether the calling goroutine is inside a Task
// callback that is not running a container builder. A builder declares the
// topology the run is built from, so only a callback declaring is misuse.
func (g *Graph) DeclaredInCallback() bool {
	if g.Executing() == 0 {
		return false
	}
	m := ReadStackMarks()
	return m.Callbacks > m.Builders
}
