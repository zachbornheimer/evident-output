package graph

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// Graph is the declared shape of a run: every Task and container with its
// identity, and the numbering that orders them. It writes truth only through
// record. Its own mutex guards its state; the lock order is graph then
// record, never the reverse, and it calls nothing outside this package and
// record while holding it.
type Graph struct {
	run   *record.Run
	idSeq atomic.Uint64

	mu         sync.Mutex
	declSeq    int
	tasks      map[string]*Task
	containers map[string]*Container
	keys       map[string]struct{}
	roots      siblings
}

// New is an empty Graph whose Tasks record into run.
func New(run *record.Run) *Graph {
	return &Graph{
		run:        run,
		tasks:      make(map[string]*Task),
		containers: make(map[string]*Container),
		keys:       make(map[string]struct{}),
	}
}

// NextID is the next id under prefix, unique across the run whatever prefix.
func (g *Graph) NextID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, g.idSeq.Add(1))
}

// NextDeclaration claims the next declaration number.
func (g *Graph) NextDeclaration() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.nextDeclarationLocked()
}

func (g *Graph) nextDeclarationLocked() int {
	g.declSeq++
	return g.declSeq
}

// Declarations is how many declaration numbers have been claimed.
func (g *Graph) Declarations() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.declSeq
}

// Task is the Task registered under id, or nil.
func (g *Graph) Task(id string) *Task {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.tasks[id]
}

// Container is the container registered under id, or nil.
func (g *Graph) Container(id string) *Container {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.containers[id]
}

// NameTaken reports whether a child of kind named name already exists under
// parent (nil is the root). Tasks and containers are separate namespaces: a
// Task and a Group may share a name, but a Group and a Sequence may not.
func (g *Graph) NameTaken(parent *Container, kind EntityKind, name string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	names := g.namesLocked(parent)
	if kind == KindTask {
		return names.taskTaken(name)
	}
	return names.containerTaken(name)
}

// ClaimName makes name taken for a child of kind under parent.
func (g *Graph) ClaimName(parent *Container, kind EntityKind, name string) {
	g.mu.Lock()
	defer g.mu.Unlock()
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
// with ClaimName once the declaration holds.
func (g *Graph) AddTask(parent *Container, name string, init record.TaskInit) *Task {
	id := g.NextID("task")
	init.ID = record.TaskID(id)
	t := &Task{
		graph: g, ID: id, Name: name, Parent: parent,
		Rec: g.run.NewTask(init),
		key: StableKey(KindTask, KeyOf(parent), name),
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	t.Declaration = g.nextDeclarationLocked()
	g.tasks[id] = t
	return t
}

// AddContainer declares a Group (sequential false) or Sequence named name
// (already DeclaredName-normalized) under parent. The name is not claimed;
// see ClaimName.
func (g *Graph) AddContainer(parent *Container, name string, sequential bool) *Container {
	c := &Container{
		ID: g.NextID("tasks"), Name: name, Sequential: sequential, Parent: parent,
		Rec: g.run.NewContainer(),
		key: StableKey(ContainerKind(sequential), KeyOf(parent), name),
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	c.Declaration = g.nextDeclarationLocked()
	g.containers[c.ID] = c
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
	g.mu.Lock()
	defer g.mu.Unlock()
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
