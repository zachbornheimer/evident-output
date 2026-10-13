package graph

import "github.com/zachbornheimer/evident-output/internal/misuse"

// ErrDeclaredInCallback is why a declaration was refused: a Task, Group, or
// Sequence was declared from inside a Task's Define callback. Declare it
// from a container builder, or before the run.
var ErrDeclaredInCallback = misuse.ErrDeclaredInCallback

// ErrInvalidConfig is why a declaration or an operation's spec was refused:
// it asks for something no run can honor, such as an Effect that claims a
// Path and a Resource at once.
var ErrInvalidConfig = misuse.ErrInvalidConfig

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
