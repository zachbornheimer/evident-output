package core

import "time"

// dependencyGraph is a Conclusion's Task dependency edges, resolved to
// Task IDs: a Task's After predecessors (a collection predecessor stands
// for every Task inside it) plus, inside a Sequence, the step before it.
type dependencyGraph struct {
	tasks    map[string]TaskSnapshot
	members  map[string][]string
	previous map[string]string
	finish   map[string]time.Duration
	visiting map[string]bool
}

func newDependencyGraph(c Conclusion) *dependencyGraph {
	g := &dependencyGraph{
		tasks:    map[string]TaskSnapshot{},
		members:  map[string][]string{},
		previous: map[string]string{},
		finish:   map[string]time.Duration{},
		visiting: map[string]bool{},
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
	var before time.Duration
	for _, pred := range g.predecessors(id) {
		before = max(before, g.finishOf(pred))
	}
	g.visiting[id] = false
	g.finish[id] = before + g.tasks[id].Timing.Running()
	return g.finish[id]
}

// predecessors is every Task id waits on.
func (g *dependencyGraph) predecessors(id string) []string {
	var preds []string
	if prev, ok := g.previous[id]; ok {
		preds = append(preds, prev)
	}
	for _, ref := range g.tasks[id].after {
		if members, isCollection := g.members[ref]; isCollection {
			preds = append(preds, members...)
			continue
		}
		preds = append(preds, ref)
	}
	return preds
}
