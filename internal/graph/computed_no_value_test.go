package graph

import (
	"errors"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// A producer can settle Done without its callback ever running (a Verify that
// found the work already satisfied), so success alone does not mean a value
// exists. Handing out the zero value there would pass a made-up answer to
// every reader.
func TestComputedReadAfterTheProducerSucceededWithoutAValueIsRefused(t *testing.T) {
	g := New(record.NewRun())
	producer := declare(g, "producer")
	computed := NewComputed[int](producer)
	submitWork(g, producer, settlingWork(g, producer, func() error { return nil }))
	g.Drain()

	got, err := computed.Read(g)

	if !errors.Is(err, ErrComputedNoValue) || got != 0 {
		t.Errorf("Read = %d, %v; want zero and ErrComputedNoValue", got, err)
	}
	if !errors.Is(err, ErrComputedUnsettled) {
		t.Errorf("Read = %v; ErrComputedNoValue must still match ErrComputedUnsettled", err)
	}
}
