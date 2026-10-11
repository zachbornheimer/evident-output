// This file owns the Define scope: the window in which a Task's callback may
// run operations, and the begin and end of each operation inside it.

package graph

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/zachbornheimer/evident-output/internal/record"
)

var (
	// ErrNoTaskContext is what ScopeOf returns for a context that never came
	// from a Define callback: a bare context.Background(), or one from
	// outside any callback (§7.1).
	ErrNoTaskContext = errors.New("evo: context carries no task scope")
	// ErrTaskClosed is what ScopeOf returns for a context captured during a
	// Define callback and reused after that callback returned (§7.1).
	ErrTaskClosed = errors.New("evo: task scope is closed")
)

// scopeKey is the context.Value key a Scope[O] travels under. It is a
// distinct type per owner type, so nothing outside this package can collide
// with or forge it, and a context scoped for one kind of owner is not
// mistaken for another's.
type scopeKey[O any] struct{}

// Scope is one Define execution: the Task it belongs to, the owner the
// operations inside it act through, and whether the callback has returned.
// O is whatever performs the operations; graph never looks inside it.
type Scope[O any] struct {
	graph  *Graph
	task   record.TaskID
	owner  O
	closed atomic.Bool
}

// NewScope opens the scope of one Define execution of task, owned by owner.
func NewScope[O any](g *Graph, task record.TaskID, owner O) *Scope[O] {
	return &Scope[O]{graph: g, task: task, owner: owner}
}

// Task is the Task whose callback this scope belongs to.
func (s *Scope[O]) Task() record.TaskID { return s.task }

// Owner is what performs the operations inside the scope.
func (s *Scope[O]) Owner() O { return s.owner }

// Close ends the scope: the Define callback has returned. A context captured
// during the callback and used again afterwards is exactly the misuse
// ErrTaskClosed reports.
func (s *Scope[O]) Close() { s.closed.Store(true) }

// WithScope derives a context carrying s, for the Define callback and the
// operations it calls to read back through ScopeOf.
func WithScope[O any](ctx context.Context, s *Scope[O]) context.Context {
	return context.WithValue(ctx, scopeKey[O]{}, s)
}

// ScopeOf returns the scope ctx's Define callback is running under.
//
//   - ErrNoTaskContext: ctx carries no scope at all.
//   - ErrTaskClosed: ctx carries a scope whose Define callback already
//     returned.
func ScopeOf[O any](ctx context.Context) (*Scope[O], error) {
	if ctx == nil {
		return nil, ErrNoTaskContext
	}
	scope, ok := ctx.Value(scopeKey[O]{}).(*Scope[O])
	if !ok || scope == nil {
		return nil, ErrNoTaskContext
	}
	if scope.closed.Load() {
		return nil, ErrTaskClosed
	}
	return scope, nil
}

// BeginOperation is the one entry every Define-scoped operation passes before
// doing anything: it finds the scope ctx belongs to, and refuses a ctx that
// is already done. A refusal reaches the MisuseSink, so a cancelled Run never
// reads as silent success whichever operation the caller used. label names
// the operation in the error ("File \"a.txt\""). The operation ends when the
// caller returns; the scope ends with Close.
func BeginOperation[O any](ctx context.Context, label string) (*Scope[O], error) {
	scope, err := ScopeOf[O](ctx)
	if err != nil {
		return nil, err
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		wrapped := fmt.Errorf("evo: %s: %w", label, ctxErr)
		scope.graph.misuse.RecordMisuseFor("", wrapped)
		return nil, wrapped
	}
	return scope, nil
}
