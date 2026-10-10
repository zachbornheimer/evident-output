package graph

import (
	"errors"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// producedComputed is a Computed whose producer settled Done with value 42.
func producedComputed(g *Graph) *Computed[int] {
	producer := declare(g, "producer")
	computed := NewComputed[int](producer)
	submitWork(g, producer, settlingWork(g, producer, func() error { computed.Set(42); return nil }))
	g.Drain()
	return computed
}

func TestComputedReadBeforeTheProducerSettledIsUnsettled(t *testing.T) {
	g := New(record.NewRun())
	computed := NewComputed[int](declare(g, "producer"))

	got, err := computed.ReadAs(g, g.CurrentReader())

	if !errors.Is(err, ErrComputedUnsettled) || got != 0 {
		t.Errorf("Read = %d, %v; want zero and ErrComputedUnsettled", got, err)
	}
}

func TestComputedReadOutsideEveryCallbackAfterTheProducerSettledIsTheValue(t *testing.T) {
	g := New(record.NewRun())
	computed := producedComputed(g)

	got, err := computed.ReadAs(g, g.CurrentReader())

	if err != nil || got != 42 {
		t.Errorf("Read = %d, %v; want 42, nil", got, err)
	}
}

func TestComputedReadFromACallbackNotOrderedAfterTheProducerIsUnordered(t *testing.T) {
	g := New(record.NewRun())
	computed := producedComputed(g)
	reader := declare(g, "unordered reader")
	var err error
	submitWork(g, reader, settlingWork(g, reader, func() error { _, err = computed.ReadAs(g, g.CurrentReader()); return nil }))

	g.Drain()

	if !errors.Is(err, ErrComputedUnordered) {
		t.Errorf("Read = %v, want ErrComputedUnordered", err)
	}
}

func TestComputedReadFromACallbackAfterTheProducerIsTheValue(t *testing.T) {
	g := New(record.NewRun())
	producer := declare(g, "producer")
	computed := NewComputed[int](producer)
	submitWork(g, producer, settlingWork(g, producer, func() error { computed.Set(7); return nil }))
	reader := declare(g, "ordered reader")
	g.AddAfter(reader, AfterTask(producer))
	var got int
	var err error
	submitWork(g, reader, settlingWork(g, reader, func() error { got, err = computed.ReadAs(g, g.CurrentReader()); return nil }))

	g.Drain()

	if err != nil || got != 7 {
		t.Errorf("Read = %d, %v; want 7, nil", got, err)
	}
}
