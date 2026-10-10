package graph

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/zachbornheimer/evident-output/internal/record"
)

var (
	// ErrEntityLimit is why AddTask refused a Task: the run already holds as
	// many as the graph was built to allow.
	ErrEntityLimit = errors.New("evo: entity limit reached")
	// ErrClosed is why AddTask refused a Task: the graph was closed.
	ErrClosed = errors.New("evo: graph is closed")
)

// MisuseSink hears the misuse the scheduler finds while it works, so the
// engine can keep recording it until misuse has a home of its own. It is
// the one callback the graph makes, besides the record's listener, and it
// is called with no graph lock held. A strict sink panics after recording.
type MisuseSink interface {
	// RecordMisuseFor reports err, naming the entity it happened on.
	RecordMisuseFor(subject string, err error)
}

// discardMisuse is the sink of a graph built without one.
type discardMisuse struct{}

func (discardMisuse) RecordMisuseFor(string, error) {}

// Option configures a Graph at construction.
type Option func(*Graph)

// WithMaxEntities limits how many Tasks the graph accepts; zero or less
// means no limit.
func WithMaxEntities(n int) Option { return func(g *Graph) { g.limit = n } }

// WithMisuseSink makes s hear the misuse the scheduler finds.
func WithMisuseSink(s MisuseSink) Option { return func(g *Graph) { g.misuse = s } }

// Graph is the declared shape of a run: every Task and container with its
// identity, the numbering that orders them, and the scheduling state that
// decides when each may start. It writes truth only through record. Its own
// mutex guards its state; the lock order is graph then record, never the
// reverse, and it calls nothing outside this package and record while
// holding it. The MisuseSink hears misuse only once the lock is free (see
// misuseNote). Its critical sections hold the record's notifications, so the
// listener hears what they wrote only once the lock is free.
type Graph struct {
	run    *record.Run
	idSeq  atomic.Uint64
	limit  int
	misuse MisuseSink
	// waitUnderClaim is the sentinel a wait refused for a held claim wraps.
	waitUnderClaim error

	mu         record.Mutex
	closed     bool
	declSeq    int
	tasks      map[string]*Task
	taskList   []*Task
	gates      []*Task
	containers map[string]*Container
	keys       map[string]struct{}
	roots      siblings
	sched      scheduler
	exec       executor
	scope      cancellationScope
	stop       runStop
	barriers   map[string]chan struct{}
}

// scheduler is the run's scheduling state: the Tasks ready to start and the
// bookkeeping that decides what may start next. Guarded by the Graph's mutex.
type scheduler struct {
	// queue holds the Tasks ready to start.
	queue queue
	// parked counts Tasks waiting off the queue on a predecessor.
	parked int
	// woken is the worklist wakeLocked drains; waking marks it in use.
	woken  []*Task
	waking bool
	// work counts submitted work that has not settled; the drain waits on it.
	work sync.WaitGroup
	// draining is set once the run starts running the queue to empty.
	draining bool
	// predChecks counts predecessor outcomes read, so a test can prove
	// fan-in scheduling stays linear.
	predChecks int
	// followerChecks counts Sequence followers examined, so a test can prove
	// repeated failures stay linear.
	followerChecks int
}

// New is an empty Graph whose Tasks record into run.
func New(run *record.Run, options ...Option) *Graph {
	g := &Graph{
		run:            run,
		misuse:         discardMisuse{},
		waitUnderClaim: ErrWaitUnderClaim,
		tasks:          make(map[string]*Task),
		containers:     make(map[string]*Container),
		keys:           make(map[string]struct{}),
		scope:          newCancellationScope(processRoot()),
	}
	g.mu.Bind(run)
	for _, apply := range options {
		apply(g)
	}
	return g
}

func (g *Graph) lock()   { g.mu.Lock() }
func (g *Graph) unlock() { g.mu.Unlock() }

// lockRead and unlockRead bracket a section that only reads the graph and
// writes nothing to the record, so it has no notifications to hold.
func (g *Graph) lockRead()   { g.mu.LockRead() }
func (g *Graph) unlockRead() { g.mu.UnlockRead() }

// Close ends the graph's lifecycle: it accepts no further Task.
func (g *Graph) Close() {
	g.lock()
	defer g.unlock()
	g.closed = true
}

// NextID is the next id under prefix, unique across the run whatever prefix.
func (g *Graph) NextID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, g.idSeq.Add(1))
}

// NextDeclaration claims the next declaration number.
func (g *Graph) NextDeclaration() int {
	g.lock()
	defer g.unlock()
	return g.nextDeclarationLocked()
}

func (g *Graph) nextDeclarationLocked() int {
	g.declSeq++
	return g.declSeq
}

// Declarations is how many declaration numbers have been claimed.
func (g *Graph) Declarations() int {
	g.lock()
	defer g.unlock()
	return g.declSeq
}

// Task is the Task registered under id, or nil.
func (g *Graph) Task(id string) *Task {
	g.lock()
	defer g.unlock()
	return g.tasks[id]
}

// Container is the container registered under id, or nil.
func (g *Graph) Container(id string) *Container {
	g.lock()
	defer g.unlock()
	return g.containers[id]
}

// NameTaken reports whether a child of kind named name already exists under
// parent (nil is the root). Tasks and containers are separate namespaces: a
// Task and a Group may share a name, but a Group and a Sequence may not.
func (g *Graph) NameTaken(parent *Container, kind EntityKind, name string) bool {
	g.lock()
	defer g.unlock()
	names := g.namesLocked(parent)
	if kind == KindTask {
		return names.taskTaken(name)
	}
	return names.containerTaken(name)
}

// ClaimName makes name taken for a child of kind under parent.
func (g *Graph) ClaimName(parent *Container, kind EntityKind, name string) {
	g.lock()
	defer g.unlock()
	names := g.namesLocked(parent)
	if kind == KindTask {
		names.claimTask(name)
		return
	}
	names.claimContainer(name)
}

func (g *Graph) namesLocked(parent *Container) *siblings {
	if parent != nil {
		return &parent.names
	}
	return &g.roots
}

// AddTask declares a Task named name (already DeclaredName-normalized) under
// parent, starting its record from init. Its stable key is kind + the
// parent's key + name; Rekey replaces it later. The name is not claimed: a
// refused declaration's row still gets an identity, so the caller claims it
// with ClaimName once the declaration holds. Under a container the Task
// starts after the step before it, becomes the container's latest step, and
// is counted by every container above; a Task declared after a failure
// stopped its Sequence step settles NotStarted at once. It is refused with
// ErrClosed after Close and with ErrEntityLimit at the graph's limit.
func (g *Graph) AddTask(parent *Container, name string, init record.TaskInit) (*Task, error) {
	g.lock()
	defer g.unlock()
	if g.closed {
		return nil, ErrClosed
	}
	if g.limit > 0 && len(g.taskList) >= g.limit {
		return nil, ErrEntityLimit
	}
	t := g.newTaskLocked(parent, name, init)
	if parent != nil {
		g.joinContainerLocked(t, parent)
		g.stopIfFollowerLocked(t)
	}
	return t, nil
}

// AddTerminalTask declares a Task the library invents to carry a run-level
// outcome rather than one the caller declared: at the root, outside the
// entity limit, and never scheduled.
func (g *Graph) AddTerminalTask(name string, init record.TaskInit) *Task {
	g.lock()
	defer g.unlock()
	return g.newTaskLocked(nil, name, init)
}

func (g *Graph) newTaskLocked(parent *Container, name string, init record.TaskInit) *Task {
	id := g.NextID("task")
	init.ID = record.TaskID(id)
	t := &Task{
		graph: g, ID: id, Name: name, Parent: parent,
		Rec:         g.run.NewTask(init),
		key:         StableKey(KindTask, KeyOf(parent), name),
		done:        make(chan struct{}),
		Declaration: g.nextDeclarationLocked(),
	}
	g.tasks[id] = t
	g.taskList = append(g.taskList, t)
	return t
}

// AddContainer declares a Group (sequential false) or Sequence named name
// (already DeclaredName-normalized) under parent. The name is not claimed;
// see ClaimName. Under a container it starts after the step before it and
// becomes that container's latest step.
func (g *Graph) AddContainer(parent *Container, name string, sequential bool) *Container {
	c := &Container{
		graph: g,
		ID:    g.NextID("tasks"), Name: name, Sequential: sequential, Parent: parent,
		Rec: g.run.NewContainer(),
		key: StableKey(ContainerKind(sequential), KeyOf(parent), name),
	}
	g.lock()
	defer g.unlock()
	c.Declaration = g.nextDeclarationLocked()
	g.containers[c.ID] = c
	if parent != nil {
		g.joinParentLocked(c, parent)
	}
	return c
}

// Rekeyed is what Rekey did with a requested key.
type Rekeyed int

const (
	// KeyReplaced means the Task now has the requested key.
	KeyReplaced Rekeyed = iota
	// KeyUnchanged means the Task already had the requested key.
	KeyUnchanged
	// KeyTaken means another Task holds the requested key; nothing changed.
	KeyTaken
)

// Rekey replaces t's stable key with key, unless another Task holds it.
func (g *Graph) Rekey(t *Task, key string) Rekeyed {
	g.lock()
	defer g.unlock()
	if key == t.key {
		return KeyUnchanged
	}
	if _, taken := g.keys[key]; taken {
		return KeyTaken
	}
	delete(g.keys, t.key)
	g.keys[key] = struct{}{}
	t.key = key
	return KeyReplaced
}
