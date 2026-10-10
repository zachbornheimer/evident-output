package engine

// settleLocked is the one way a Task reaches a terminal state: it hands the
// Task to the graph (see graph.Graph.Settle), which owns the scheduling
// bookkeeping a terminal transition owes. What the render state owes it (the
// snapshot version, the task.<state> event, the live census and filing, the
// plain heartbeat) is the engine listener's job: it hears the transition
// once the lock is free (see outputListener).
//
// Callers own only what differs between paths: the summary, the Problems,
// and where the settled row is committed. Callers must already hold o.mu.
func (o *Output) settleLocked(st *taskState, state EntityState) {
	o.graph.Settle(st.node, state)
}
