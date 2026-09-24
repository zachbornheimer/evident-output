package core

import "time"

// dependencyGraph is a Conclusion's Task dependency edges: a Task's After
// predecessors (Tasks, or collections standing for every Task inside
// them) plus, inside a Sequence, the step before it. A collection's finish
// is computed once and shared by every dependent, so a Group After a
// Group costs O(N+M), not O(N×M).
type dependencyGraph struct {
	tasks     map[string]TaskSnapshot
	members   map[string][]string
	previous  map[string]string
	finish    map[string]time.Duration
	colFinish map[string]time.Duration
	visiting  map[string]bool
}

func newDependencyGraph(c Conclusion) *dependencyGraph {
	g := &dependencyGraph{
		tasks:     map[string]TaskSnapshot{},
		members:   map[string][]string{},
		previous:  map[string]string{},
		finish:    map[string]time.Duration{},
		colFinish: map[string]time.Duration{},
		visiting:  map[string]bool{},
	}
	for _, t := range c.Tasks {
		g.tasks[t.ID] = t
	}
	for _, col := range c.Collections {
		g.addCollection(col)
	}
	return g
}

// addCollection indexes col's Tasks, its Sequence order, and its member
// list, recursively, returning every Task ID inside it.
func (g *dependencyGraph) addCollection(col TasksSnapshot) []string {
	var ids []string
	for i, t := range col.Tasks {
		g.tasks[t.ID] = t
		ids = append(ids, t.ID)
		if col.Sequential && i > 0 {
			g.previous[t.ID] = col.Tasks[i-1].ID
		}
	}
	for _, child := range col.Collections {
		ids = append(ids, g.addCollection(child)...)
	}
	g.members[col.ID] = ids
	return ids
}

// criticalPath is the longest chain of Running time ending at any Task.
func (g *dependencyGraph) criticalPath() time.Duration {
	var longest time.Duration
	for id := range g.tasks {
		longest = max(longest, g.finishOf(id))
	}
	return longest
}

// finishOf is the longest chain of Running time ending with id: its own
// Running plus the longest chain among its predecessors. A dependency
// cycle (a deadlocked declaration) contributes nothing past the repeat.
func (g *dependencyGraph) finishOf(id string) time.Duration {
	if d, done := g.finish[id]; done {
		return d
	}
	if g.visiting[id] {
		return 0
	}
	g.visiting[id] = true
	before := g.longestBefore(id)
	g.visiting[id] = false
	g.finish[id] = before + g.tasks[id].Timing.Running()
	return g.finish[id]
}

// longestBefore is the longest chain among every predecessor id waits on.
func (g *dependencyGraph) longestBefore(id string) time.Duration {
	var before time.Duration
	if prev, ok := g.previous[id]; ok {
		before = g.finishOf(prev)
	}
	for _, ref := range g.tasks[id].after {
		if members, isCollection := g.members[ref]; isCollection {
			before = max(before, g.collectionFinish(ref, members))
			continue
		}
		before = max(before, g.finishOf(ref))
	}
	return before
}

// collectionFinish is the longest chain ending at any of a collection's
// members. It is memoized unless a member is mid-evaluation (a Task After
// its own collection), where the partial answer must not be reused.
func (g *dependencyGraph) collectionFinish(id string, members []string) time.Duration {
	if d, done := g.colFinish[id]; done {
		return d
	}
	var d time.Duration
	cyclic := false
	for _, m := range members {
		cyclic = cyclic || g.visiting[m]
		d = max(d, g.finishOf(m))
	}
	if !cyclic {
		g.colFinish[id] = d
	}
	return d
}
