package engine

import (
	"strings"

	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// depNode is one vertex of the run's waits-for graph: a Task or a
// Group/Sequence.
type depNode struct {
	task *taskState
	col  *tasksState
}

func (n depNode) name() string {
	if n.task != nil {
		return n.task.name
	}
	return n.col.name
}

// waitsForLocked lists what n is still waiting for. A parked Task waits for
// its pending predecessors; a collection waits for its unresolved members.
// A Task that is not parked waits on nothing the graph can see.
func (o *Output) waitsForLocked(n depNode) []depNode {
	var out []depNode
	if n.task != nil {
		if n.task.sched.phase != phaseParked {
			return nil
		}
		for _, p := range n.task.sched.preds {
			if outcome, _ := o.outcomeLocked(p); outcome == predPending {
				out = append(out, depNode(p))
			}
		}
		return out
	}
	if n.col.tally.total == 0 {
		// An empty collection waits for what it starts after.
		for _, p := range n.col.entry {
			if outcome, _ := o.outcomeLocked(p); outcome == predPending {
				out = append(out, depNode(p))
			}
		}
		return out
	}
	for _, t := range n.col.tasks {
		if !core.IsTerminalTask(t.state) {
			out = append(out, depNode{task: t})
		}
	}
	for _, c := range n.col.children {
		if outcome, _ := o.collectionOutcomeLocked(c); outcome == predPending {
			out = append(out, depNode{col: c})
		}
	}
	return out
}

// dependencyCyclesLocked finds the cycles among parked Tasks' waits-for
// edges with one iterative depth-first walk: each cycle is its vertices in
// order, the first repeated at the end.
func (o *Output) dependencyCyclesLocked() [][]depNode {
	const (
		unvisited = iota
		onPath
		finished
	)
	color := make(map[depNode]uint8)
	var cycles [][]depNode
	for _, st := range o.tasks {
		root := depNode{task: st}
		if st.sched.phase != phaseParked || color[root] != unvisited {
			continue
		}
		color[root] = onPath
		path := []cycleFrame{{n: root, edges: o.waitsForLocked(root)}}
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
				path = append(path, cycleFrame{n: to, edges: o.waitsForLocked(to)})
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

// blockCyclesLocked settles Blocked every parked Task caught in an After
// cycle — it can never start, and the row says why — and records the
// misuse once per cycle. Its dependents then settle NotStarted through the
// ordinary cascade. It reports whether it found any cycle.
func (o *Output) blockCyclesLocked() bool {
	if o.sched.parked == 0 {
		return false
	}
	cycles := o.dependencyCyclesLocked()
	// Every Task in a cycle is Blocked, so none may cascade NotStarted into
	// another before its own turn.
	release := o.holdWakesLocked()
	defer release()
	for _, cycle := range cycles {
		names := make([]string, len(cycle))
		for i, n := range cycle {
			names[i] = n.name()
		}
		path := strings.Join(names, " → ")
		for _, n := range cycle {
			if st := n.task; st != nil && st.sched.phase == phaseParked && !core.IsTerminalTask(st.state) {
				o.blockInCycleLocked(st, path)
			}
		}
		o.recordMisuseFor(path, errDependencyCycle)
	}
	return len(cycles) > 0
}

func (o *Output) blockInCycleLocked(st *taskState, path string) {
	summary := txt.Text("dependency cycle: " + path)
	st.summary = summary
	st.problems = core.StoreProblems([]Problem{{Subject: st.name, Summary: summary}})
	o.settleLocked(st, Blocked)
}
