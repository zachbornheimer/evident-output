package graph

import (
	"slices"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// builderPhase is where a container's topology builder stands.
type builderPhase uint8

const (
	// builderPending: Defined, and not finished: waiting on the
	// container's predecessors, or running.
	builderPending builderPhase = iota
	// builderDone: the builder returned; its children were declared.
	builderDone
	// builderNotStarted: a predecessor can never succeed, so the builder
	// never ran.
	builderNotStarted
	// builderFailed: the builder panicked before declaring every child.
	builderFailed
)

// builder is the declaration work a Group or Sequence deferred until its
// predecessors succeed. gate is the scheduler's entity for that work: it
// waits on the container's After edges like a Task does, but it is no row,
// and no collection counts it.
type builder struct {
	gate  *Task
	phase builderPhase
}

// AddGate defers c's children: the returned gate runs work once every After
// predecessor of c succeeded, and the membership of c and of every container
// above it stays open until the gate concludes. It returns nil, changing
// nothing, once c has a builder or a member. The gate still has to be
// enqueued and placed.
func (g *Graph) AddGate(c *Container, work func() error) *Task {
	g.lock()
	defer g.unlock()
	if c.builder != nil || len(c.tasks)+len(c.children) > 0 {
		return nil
	}
	id := g.NextID("gate")
	gate := &Task{
		graph: g, ID: id, Name: c.Name,
		Declaration: g.nextDeclarationLocked(),
		Rec:         g.run.NewTask(record.TaskInit{ID: record.TaskID(id), State: record.Pending}),
		gateFor:     c,
		done:        make(chan struct{}),
	}
	gate.sched.preds = slices.Clone(c.entry)
	gate.sched.work = Work{Run: work}
	c.builder = &builder{gate: gate, phase: builderPending}
	for p := c; p != nil; p = p.Parent {
		p.tally.holds++
	}
	g.gates = append(g.gates, gate)
	return gate
}

// BuilderGate is the gate of c's deferred declaration work, nil when c has
// none.
func (c *Container) BuilderGate() *Task {
	c.graph.lock()
	defer c.graph.unlock()
	if c.builder == nil {
		return nil
	}
	return c.builder.gate
}

// BuilderFailed reports whether c's builder panicked before declaring every
// child.
func (c *Container) BuilderFailed() bool {
	c.graph.lockRead()
	defer c.graph.unlockRead()
	return c.builder != nil && c.builder.phase == builderFailed
}

// BuilderNotStarted reports whether c's builder never ran because a
// predecessor can never succeed.
func (c *Container) BuilderNotStarted() bool {
	c.graph.lockRead()
	defer c.graph.unlockRead()
	return c.builder != nil && c.builder.phase == builderNotStarted
}

// concludeGateLocked is settle's tail for a gate: it records the builder's
// outcome, releases the membership holds the pending builder placed on its
// container and ancestors, and places every Task that was waiting on them.
func (g *Graph) concludeGateLocked(gate *Task) {
	c, state := gate.gateFor, gate.Rec.State()
	switch state {
	case record.Done:
		c.builder.phase = builderDone
	case record.NotStarted:
		c.builder.phase = builderNotStarted
	default:
		c.builder.phase = builderFailed
	}
	gate.closeDoneLocked()
	if gate.sched.awaitingStart() {
		g.abandonLocked(gate)
	}
	var woken []*Task
	for p := c; p != nil; p = p.Parent {
		if state != record.Done {
			woken = append(woken, p.tally.failBuilder()...)
		}
		woken = append(woken, p.tally.release(p == c, g.declSeq)...)
	}
	g.wakeLocked(woken)
}
