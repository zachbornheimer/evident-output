// This file owns declaration: a Task, Group or Sequence entering the graph,
// the refusals a declaration can meet, and a container's deferred builder.
// Every declaration runs under the run's lock, which its caller holds.

package graph

import (
	"errors"
	"fmt"

	"github.com/zachbornheimer/evident-output/internal/misuse"
	"github.com/zachbornheimer/evident-output/internal/record"
)

// DeclareTask declares a Task named name under parent (nil: at the root).
// A repeated name is a duplicate sibling (§3.1), never a get-or-create:
// letting two declarations merge into one identity would make a false
// "already satisfied" possible once identity drives the manifest. The
// duplicate still declares a row, settled Failed with
// record.CodeDuplicateSiblingName, so the reader sees what went wrong; the
// error is what the caller's refused handle keeps. The caller holds the
// run's lock.
func (g *Graph) DeclareTask(parent *Container, name string) (*Task, error) {
	name = DeclaredName(name)
	if g.NameTaken(parent, KindTask, name) {
		return nil, g.refuseDuplicate(parent, KindTask, name)
	}
	t, err := g.declareTaskRow(parent, name)
	if err == nil {
		g.ClaimName(parent, KindTask, name)
	}
	return t, err
}

// DeclareContainer declares a Group (sequential false) or Sequence named
// name under parent (nil: at the root). Under a Sequence it starts after the
// step before it. A repeated name is refused as DeclareTask's is. The caller
// holds the run's lock.
func (g *Graph) DeclareContainer(parent *Container, name string, sequential bool) (*Container, error) {
	name = DeclaredName(name)
	kind := ContainerKind(sequential)
	if g.NameTaken(parent, kind, name) {
		return nil, g.refuseDuplicate(parent, kind, name)
	}
	if err := g.admitDeclaration(); err != nil {
		return nil, err
	}
	c := g.AddContainer(parent, name, sequential)
	g.ClaimName(parent, kind, name)
	g.host.ContainerDeclared(c)
	return c, nil
}

// admitDeclaration refuses, as misuse, a declaration once the run is closing
// or from inside a Task's callback.
func (g *Graph) admitDeclaration() error {
	if err := g.host.CheckOpen(); err != nil {
		return err
	}
	if g.DeclaredInCallback() {
		g.misuse.RecordMisuseFor("", misuse.ErrDeclaredInCallback)
		return misuse.ErrDeclaredInCallback
	}
	return nil
}

// declareTaskRow declares a Task's row without claiming its name.
func (g *Graph) declareTaskRow(parent *Container, name string) (*Task, error) {
	if err := g.admitDeclaration(); err != nil {
		return nil, err
	}
	t, err := g.AddTask(parent, name, record.TaskInit{State: record.Pending, Progress: record.Progress{Kind: record.Indeterminate}})
	if err != nil {
		err = declarationRefusal(err)
		g.misuse.RecordMisuseFor("", err)
		return nil, err
	}
	g.host.TaskDeclared(t)
	return t, nil
}

// declarationRefusal is the misuse a refused AddTask records.
func declarationRefusal(err error) error {
	switch {
	case errors.Is(err, ErrEntityLimit):
		return misuse.ErrLimitExceeded
	case errors.Is(err, ErrClosed):
		return misuse.ErrClosed
	default:
		return fmt.Errorf("declaring a Task: %w", err)
	}
}

// refuseDuplicate declares the row of a duplicate sibling, settled Failed
// with the Problem that names it (`✗ t  duplicate task name`), records the
// misuse, and returns the refusal the duplicate's handle keeps.
func (g *Graph) refuseDuplicate(parent *Container, kind EntityKind, name string) error {
	refused := fmt.Errorf("%w: %s", misuse.ErrDuplicateSiblingName, name)
	t, err := g.declareTaskRow(parent, name)
	if err != nil {
		return refused
	}
	t.Rec.AppendProblems(record.Problem{
		Code:    record.CodeDuplicateSiblingName,
		Summary: fmt.Sprintf("duplicate %s name", kind),
	})
	g.Settle(t, record.Failed)
	g.host.Changed(t)
	g.misuse.RecordMisuseFor(name, misuse.ErrDuplicateSiblingName)
	return refused
}

// DefineBuilder defers c's children to build (§2): it runs once, when
// every predecessor c was declared After succeeded, and never once one of
// them cannot. It is misuse once the run is closing, and on a container
// that already has a builder or children. It takes the run's lock itself.
func (g *Graph) DefineBuilder(c *Container, build func()) {
	h := g.host
	h.Lock()
	if h.CheckOpen() != nil {
		h.Unlock()
		return
	}
	gate := g.AddGate(c, func() error { build(); return nil })
	if gate == nil {
		h.Unlock()
		g.misuse.RecordMisuseFor("", misuse.ErrInvalidConfig)
		return
	}
	h.GateDeclared(gate)
	g.Submit(gate, Work{})
	h.Changed(gate)
	h.Unlock()
	g.Kick()
}

// After declares t's predecessors (see AddAfter); an edge After refuses is
// misuse. The caller holds the run's lock.
func (g *Graph) After(t *Task, preds ...Predecessor) {
	if !g.AddAfter(t, preds...) {
		g.misuse.RecordMisuseFor("", misuse.ErrInvalidConfig)
	}
}

// ContainerAfter declares c's predecessors (see AddContainerAfter); an edge
// it refuses is misuse. The caller holds the run's lock.
func (g *Graph) ContainerAfter(c *Container, preds ...Predecessor) {
	if !g.AddContainerAfter(c, preds...) {
		g.misuse.RecordMisuseFor("", misuse.ErrInvalidConfig)
	}
}
