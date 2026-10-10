package graph

import (
	"errors"
	"strings"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// ErrDependencyCycle is what the MisuseSink hears when After edges close a
// cycle: those Tasks can never start, and each row says so.
var ErrDependencyCycle = errors.New("evo: After dependency cycle")

// depNode is one vertex of the run's waits-for graph: a Task or a
// Group/Sequence.
type depNode struct {
	task *Task
	col  *Container
}

func (n depNode) name() string {
	if n.task != nil {
		return n.task.Name
	}
	return n.col.Name
}

// waitsForLocked lists what n is still waiting for. A parked Task waits for
// its pending predecessors; a collection waits for its unresolved members.
// A Task that is not parked waits on nothing the graph can see.
func (g *Graph) waitsForLocked(n depNode) []depNode {
	var out []depNode
	if n.task != nil {
		if n.task.sched.phase != PhaseParked {
			return nil
		}
		for _, p := range n.task.sched.preds {
			if outcome, _ := g.outcomeLocked(p); outcome == predPending {
				out = append(out, depNode{task: p.task, col: p.col})
			}
		}
		return out
	}
	if n.col.tally.total() == 0 {
		// An empty collection waits for what it starts after.
		for _, p := range n.col.entry {
			if outcome, _ := g.outcomeLocked(p); outcome == predPending {
				out = append(out, depNode{task: p.task, col: p.col})
			}
		}
		return out
	}
	for _, t := range n.col.tasks {
		if !record.IsTerminalTask(t.Rec.State()) {
			out = append(out, depNode{task: t})
		}
	}
	for _, c := range n.col.children {
		if outcome, _ := g.collectionOutcomeLocked(Predecessor{col: c}); outcome == predPending {
			out = append(out, depNode{col: c})
		}
	}
	return out
}

// dependencyCyclesLocked finds the cycles among parked Tasks' waits-for
// edges with one iterative depth-first walk: each cycle is its vertices in
// order, the first repeated at the end.
func (g *Graph) dependencyCyclesLocked() [][]depNode {
	const (
		unvisited = iota
		onPath
		finished
	)
	color := make(map[depNode]uint8)
	var cycles [][]depNode
	for _, t := range g.taskList {
		root := depNode{task: t}
		if t.sched.phase != PhaseParked || color[root] != unvisited {
			continue
		}
		color[root] = onPath
		path := []cycleFrame{{n: root, edges: g.waitsForLocked(root)}}
		for len(path) > 0 {
			top := &path[len(path)-1]
			if top.next == len(top.edges) {
				color[top.n] = finished
				path = path[:len(path)-1]
				continue
			}
			to := top.edges[top.next]
			top.next++
			switch color[to] {
			case unvisited:
				color[to] = onPath
				path = append(path, cycleFrame{n: to, edges: g.waitsForLocked(to)})
			case onPath:
				cycles = append(cycles, cycleThrough(path, to))
			}
		}
	}
	return cycles
}

// cycleFrame is one vertex on the walk's current path, with the edges it
// has left to follow.
type cycleFrame struct {
	n     depNode
	edges []depNode
	next  int
}

// cycleThrough returns the cycle closed by an edge from the path's top back
// to to, which is on the path.
func cycleThrough(path []cycleFrame, to depNode) []depNode {
	start := len(path) - 1
	for path[start].n != to {
		start--
	}
	cycle := make([]depNode, 0, len(path)-start+1)
	for _, f := range path[start:] {
		cycle = append(cycle, f.n)
	}
	return append(cycle, to)
}

// BlockCycles settles Blocked every parked Task caught in an After cycle —
// it can never start, and the row says why — and tells the MisuseSink once
// per cycle. Its dependents then settle NotStarted through the ordinary
// cascade. It reports whether it found any cycle.
func (g *Graph) BlockCycles() bool {
	g.lock()
	defer g.unlock()
	return g.blockCyclesLocked()
}

func (g *Graph) blockCyclesLocked() bool {
	if g.sched.parked == 0 {
		return false
	}
	cycles := g.dependencyCyclesLocked()
	// Every Task in a cycle is Blocked, so none may cascade NotStarted into
	// another before its own turn.
	release := g.holdWakesLocked()
	defer release()
	for _, cycle := range cycles {
		names := make([]string, len(cycle))
		for i, n := range cycle {
			names[i] = n.name()
		}
		path := strings.Join(names, " → ")
		for _, n := range cycle {
			if t := n.task; t != nil && t.sched.phase == PhaseParked && !record.IsTerminalTask(t.Rec.State()) {
				g.blockInCycleLocked(t, path)
			}
		}
		g.misuse.RecordMisuseFor(path, ErrDependencyCycle)
	}
	return len(cycles) > 0
}

// blockInCycleLocked settles t Blocked the way Block does: one Problem
// with no Subject of its own, so the row states the cycle once rather
// than again on a child line under the Task's own name.
func (g *Graph) blockInCycleLocked(t *Task, path string) {
	summary := record.SanitizeText("dependency cycle: " + path)
	t.Rec.SetSummary(summary)
	t.Rec.AppendProblems(record.Problem{Summary: summary})
	g.settleLocked(t, record.Blocked)
}
