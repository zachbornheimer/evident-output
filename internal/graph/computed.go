package graph

import (
	"errors"
	"fmt"

	"github.com/zachbornheimer/evident-output/internal/record"
)

var (
	// ErrComputedUnsettled is why a Computed read was refused: the Task that
	// produces the value has not settled successfully.
	ErrComputedUnsettled = errors.New("evo: Computed read before its Task settled")
	// ErrComputedUnordered is why a Computed read was refused: the reader is
	// a Task or container builder that is not ordered after the producing Task.
	ErrComputedUnordered = errors.New("evo: Computed read without an After edge or Sequence order to its Task")
	// ErrComputedNoValue is why a Computed read was refused: the producing
	// Task succeeded without its callback producing a value, as when a Verify
	// found the work already satisfied. It matches ErrComputedUnsettled too:
	// to the reader the value has not been produced either way.
	ErrComputedNoValue = fmt.Errorf("%w: its Task succeeded without producing a value (already satisfied, so its callback never ran)", ErrComputedUnsettled)
)

// Computed is the value one Task produces for the Tasks and containers
// ordered after it. The graph owns the rule for when it may be read: the
// producer settled successfully, and the reader can only run once it did.
type Computed[T any] struct {
	producer *Task
	value    T
	// produced is whether Set ran: a producer can succeed without its
	// callback running at all.
	produced bool
}

// NewComputed is the value producer will produce.
func NewComputed[T any](producer *Task) *Computed[T] {
	return &Computed[T]{producer: producer}
}

// Producer is the Task that produces the value.
func (c *Computed[T]) Producer() *Task { return c.producer }

// Set stores the produced value. The producer's callback calls it before the
// Task settles, and Read hands it out only after, so a read is never a data
// race.
func (c *Computed[T]) Set(v T) { c.value, c.produced = v, true }

// Read is the produced value, or why it may not be read: first
// ErrComputedUnordered when the caller is a callback or builder the declared
// order does not put after the producer, then ErrComputedUnsettled before the
// producer settled successfully, then ErrComputedNoValue when it succeeded
// without producing one. Order is declared, so it is judged before
// settledness, which a race decides: an unordered reader fails the same way
// whether or not the producer happened to settle first. Whoever runs on a
// goroutine outside every callback is ordered by definition.
func (c *Computed[T]) Read(g *Graph) (T, error) {
	var zero T
	if consumer := g.CurrentConsumer(); consumer != nil && !g.OrderedAfter(consumer, c.producer) {
		return zero, ErrComputedUnordered
	}
	if !record.DeclaresSuccess(c.producer.Rec.State()) {
		return zero, ErrComputedUnsettled
	}
	if !c.produced {
		return zero, ErrComputedNoValue
	}
	return c.value, nil
}
