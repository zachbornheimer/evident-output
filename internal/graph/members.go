package graph

// TaskCount is how many Tasks were declared directly in c.
func (c *Container) TaskCount() int {
	c.graph.lockRead()
	defer c.graph.unlockRead()
	return len(c.tasks)
}

// ChildCount is how many Groups and Sequences were declared directly in c.
func (c *Container) ChildCount() int {
	c.graph.lockRead()
	defer c.graph.unlockRead()
	return len(c.children)
}

// TaskAt is the Task declared directly in c at position i, in declaration
// order.
func (c *Container) TaskAt(i int) *Task {
	c.graph.lockRead()
	defer c.graph.unlockRead()
	return c.tasks[i]
}

// Tasks are the Tasks declared directly in c, in declaration order. The
// slice is the caller's.
func (c *Container) Tasks() []*Task {
	c.graph.lockRead()
	defer c.graph.unlockRead()
	return append([]*Task(nil), c.tasks...)
}

// Children are the Groups and Sequences declared directly in c, in
// declaration order. The slice is the caller's.
func (c *Container) Children() []*Container {
	c.graph.lockRead()
	defer c.graph.unlockRead()
	return append([]*Container(nil), c.children...)
}
