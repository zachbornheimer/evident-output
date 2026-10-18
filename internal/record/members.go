package record

// Member is one entry of a collection: a Task or a nested collection, never
// both.
type Member struct {
	Task       *TaskSnapshot
	Collection *TasksSnapshot
}

// Members are the Tasks and nested collections of c in the order they were
// declared. Tasks and Collections are kept apart for the walks that need
// only one kind; this is the one place that interleaves them, by the run's
// single declaration counter. Members declared at the same position (a view
// that carries no counter) list Tasks first.
func (c TasksSnapshot) Members() []Member {
	out := make([]Member, 0, len(c.Tasks)+len(c.Collections))
	t, k := 0, 0
	for t < len(c.Tasks) || k < len(c.Collections) {
		if k == len(c.Collections) || (t < len(c.Tasks) && c.Tasks[t].Declaration <= c.Collections[k].Declaration) {
			out = append(out, Member{Task: &c.Tasks[t]})
			t++
			continue
		}
		out = append(out, Member{Collection: &c.Collections[k]})
		k++
	}
	return out
}
