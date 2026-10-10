package graph

import (
	"errors"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// Order is declared, not observed: a callback the declared order does not put
// after the producer may not read it, whether or not the producer happened to
// settle first. Answering ErrComputedUnsettled instead let a caller treat the
// refusal as "try again later".
func TestComputedReadFromAnUnorderedCallbackBeforeTheProducerSettledIsUnordered(t *testing.T) {
	g := New(record.NewRun())
	computed := NewComputed[int](declare(g, "producer"))
	reader := declare(g, "unordered reader")
	var got int
	var err error
	submitWork(g, reader, settlingWork(g, reader, func() error { got, err = computed.Read(g); return nil }))

	g.Drain()

	if !errors.Is(err, ErrComputedUnordered) || got != 0 {
		t.Errorf("Read = %d, %v; want zero and ErrComputedUnordered", got, err)
	}
}
