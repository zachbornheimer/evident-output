package core

import (
	"slices"
	"time"
)

// dependencyGraph is a Conclusion's Task dependency edges: a Task's After
// predecessors (Tasks, or collections standing for every Task inside
// them) plus, inside a Sequence, the step before it. A collection is a
// vertex of its own with an edge to each direct child, so a Group After a
// Group costs O(N+M), not O(N×M).
//
// A dependency cycle (a deadlocked declaration, such as a Task After its
// own Group) is one strongly connected component: its Tasks count once, as
// a unit, so the critical path does not depend on where a walk enters the
// cycle.
type dependencyGraph struct {
	ids     map[vertex]int
	running []time.Duration
	edges   [][]int
	// collections marks the IDs that name a collection, so an After ref
	// resolves the way the engine resolves it.
	collections map[string]bool
}

// vertex is a Task or a collection. Nothing keeps their IDs apart, so the
// kind is part of the key.
type vertex struct {
	id         string
	collection bool
}

func newDependencyGraph(c Conclusion) *dependencyGraph {
	g := &dependencyGraph{ids: map[vertex]int{}, collections: map[string]bool{}}
	tasks := slices.Clone(c.Tasks)
	for _, col := range c.Collections {
		tasks = g.addCollection(col, tasks)
	}
	for _, t := range tasks {
		g.addTask(t)
	}
	return g
}

// vertexOf returns v's dense index, adding it on first sight.
func (g *dependencyGraph) vertexOf(v vertex) int {
	if i, ok := g.ids[v]; ok {
		return i
	}
	i := len(g.running)
	g.ids[v] = i
	g.running = append(g.running, 0)
	g.edges = append(g.edges, nil)
	return i
}

func (g *dependencyGraph) addEdge(from, to vertex) {
	f, t := g.vertexOf(from), g.vertexOf(to)
	g.edges[f] = append(g.edges[f], t)
}

// addCollection records col's vertex, its edges to its direct children,
// and its Sequence order, recursively, appending every Task inside it.
func (g *dependencyGraph) addCollection(col TasksSnapshot, tasks []TaskSnapshot) []TaskSnapshot {
	v := vertex{id: col.ID, collection: true}
	g.collections[col.ID] = true
	g.vertexOf(v)
	for i, t := range col.Tasks {
		g.addEdge(v, vertex{id: t.ID})
		if col.Sequential && i > 0 {
			g.addEdge(vertex{id: t.ID}, vertex{id: col.Tasks[i-1].ID})
		}
		tasks = append(tasks, t)
	}
	for _, child := range col.Collections {
		g.addEdge(v, vertex{id: child.ID, collection: true})
		tasks = g.addCollection(child, tasks)
	}
	return tasks
}

// addTask records t's Running time and its After edges.
func (g *dependencyGraph) addTask(t TaskSnapshot) {
	v := vertex{id: t.ID}
	g.running[g.vertexOf(v)] = t.Timing.Running()
	for _, ref := range t.after {
		g.addEdge(v, vertex{id: ref, collection: g.collections[ref]})
	}
}

// criticalPath is the longest chain of Running time ending at any Task.
func (g *dependencyGraph) criticalPath() time.Duration {
	var longest time.Duration
	for _, d := range newComponentWalk(g).finishes() {
		longest = max(longest, d)
	}
	return longest
}

// unvisited marks a vertex the component walk has not reached yet.
const unvisited = -1

// componentWalk finds the graph's strongly connected components (Tarjan)
// and, as each one closes, its finish: the Running time of every Task in
// it plus the longest finish among the components it waits on. Tarjan
// closes a component only after every component it reaches, so each
// predecessor's finish is known when it is read.
type componentWalk struct {
	g         *dependencyGraph
	next      int
	index     []int
	low       []int
	onStack   []bool
	stack     []int
	component []int
	finish    []time.Duration
}

func newComponentWalk(g *dependencyGraph) *componentWalk {
	n := len(g.running)
	return &componentWalk{
		g:         g,
		index:     slices.Repeat([]int{unvisited}, n),
		low:       make([]int, n),
		onStack:   make([]bool, n),
		component: make([]int, n),
	}
}

// finishes returns every component's finish.
func (w *componentWalk) finishes() []time.Duration {
	for v := range w.index {
		if w.index[v] == unvisited {
			w.visit(v)
		}
	}
	return w.finish
}

func (w *componentWalk) visit(v int) {
	w.index[v], w.low[v] = w.next, w.next
	w.next++
	w.stack = append(w.stack, v)
	w.onStack[v] = true
	for _, p := range w.g.edges[v] {
		if w.index[p] == unvisited {
			w.visit(p)
			w.low[v] = min(w.low[v], w.low[p])
		} else if w.onStack[p] {
			w.low[v] = min(w.low[v], w.index[p])
		}
	}
	if w.low[v] == w.index[v] {
		w.close(v)
	}
}

// close pops the component rooted at root and records its finish.
func (w *componentWalk) close(root int) {
	id := len(w.finish)
	start := len(w.stack) - 1
	for w.stack[start] != root {
		start--
	}
	members := w.stack[start:]
	for _, m := range members {
		w.onStack[m] = false
		w.component[m] = id
	}
	var own, before time.Duration
	for _, m := range members {
		own += w.g.running[m]
		for _, p := range w.g.edges[m] {
			if c := w.component[p]; c != id {
				before = max(before, w.finish[c])
			}
		}
	}
	w.stack = w.stack[:start]
	w.finish = append(w.finish, before+own)
}
