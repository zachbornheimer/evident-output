package graph

import (
	"errors"

	"github.com/zachbornheimer/evident-output/internal/record"
)

var (
	// ErrComputedUnsettled is why a Computed read was refused: the Task that
	// produces the value has not settled successfully, or settled without
	// producing one.
	ErrComputedUnsettled = errors.New("evo: Computed read before its Task settled")
	// ErrComputedUnordered is why a Computed read was refused: the reader is
	// a Task or container builder that is not ordered after the producing Task.
	ErrComputedUnordered = errors.New("evo: Computed read without an After edge or Sequence order to its Task")
)

// Computed is the value one Task produces for the Tasks and containers
// ordered after it. The graph owns the rule for when it may be read: the
// producer settled successfully, and the reader can only run once it did.
type Computed[T any] struct {
	producer *Task
	value    T
	// produced is whether Set ran.
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
// order does not put after the producer, then ErrComputedUnsettled until the
// producer settled successfully with a value. Order is declared, so it is
// judged before settledness, which a race decides: an unordered reader fails
// the same way whether or not the producer happened to settle first. A
// goroutine a callback started reads as that callback. Whoever runs on a
// goroutine outside every callback is ordered by definition.
//
// A refused read from such a started goroutine also fails the callback's own
// Task, once the callback returns: the goroutine cannot unwind it.
func (c *Computed[T]) Read(g *Graph) (T, error) {
	var zero T
	reader := g.CurrentReader()
	if err := c.refusalFor(g, reader.Task); err != nil {
		g.refuseSpawnedRead(reader, err)
		return zero, err
	}
	return c.value, nil
}

// refusalFor is why reader may not read the value, nil when it may.
func (c *Computed[T]) refusalFor(g *Graph, reader *Task) error {
	if reader != nil && !g.OrderedAfter(reader, c.producer) {
		return ErrComputedUnordered
	}
	if !record.DeclaresSuccess(c.producer.Rec.State()) || !c.produced {
		return ErrComputedUnsettled
	}
	return nil
}
