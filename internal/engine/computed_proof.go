package engine

// currentConsumerLocked is the Task or builder gate whose callback the
// calling goroutine is running, or nil for a caller outside any callback.
func (o *Output) currentConsumerLocked() *taskState {
	node := o.graph.CurrentConsumer()
	if node == nil {
		return nil
	}
	return o.taskStates[node.ID]
}

// orderedAfterLocked reports whether consumer can only run once producer
// settled (see graph.Graph.OrderedAfter).
func (o *Output) orderedAfterLocked(consumer, producer *taskState) bool {
	return o.graph.OrderedAfter(consumer.node, producer.node)
}
