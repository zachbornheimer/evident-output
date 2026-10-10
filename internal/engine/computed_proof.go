package engine

import "github.com/zachbornheimer/evident-output/internal/graph"

// consumerScope names the Task (or container builder gate) whose callback
// the calling goroutine is running, so Computed.Get can ask whether that
// consumer is ordered after the producer. Go has no goroutine-local
// storage and Get takes no context, so the scheduler records it per
// goroutine.
func (o *Output) enterConsumer(st *taskState) (leave func()) {
	g := graph.CurrentGoroutine()
	o.mu.Lock()
	if o.sched.consumers == nil {
		o.sched.consumers = make(map[graph.GoroutineID][]*taskState)
	}
	o.sched.consumers[g] = append(o.sched.consumers[g], st)
	o.mu.Unlock()
	return func() {
		o.mu.Lock()
		stack := o.sched.consumers[g]
		if len(stack) <= 1 {
			delete(o.sched.consumers, g)
		} else {
			o.sched.consumers[g] = stack[:len(stack)-1]
		}
		o.mu.Unlock()
	}
}

// currentConsumerLocked is the Task or builder gate running on the calling
// goroutine, or nil for a caller outside any callback.
func (o *Output) currentConsumerLocked() *taskState {
	stack := o.sched.consumers[graph.CurrentGoroutine()]
	if len(stack) == 0 {
		return nil
	}
	return stack[len(stack)-1]
}

// orderedAfterLocked reports whether consumer can only run once producer
// settled (see graph.Graph.OrderedAfter).
func (o *Output) orderedAfterLocked(consumer, producer *taskState) bool {
	return o.graph.OrderedAfter(consumer.node, producer.node)
}
