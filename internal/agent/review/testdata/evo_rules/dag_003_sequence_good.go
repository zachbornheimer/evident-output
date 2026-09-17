// Fixture: EVO-DAG-003 must stay silent. "producer" and "consumer" are
// declared under one evo.Sequence, in order, with no explicit .After —
// spec §5 says Sequence already creates predecessor dependencies
// automatically, so the producer/consumer relationship is already ordered.
package dag003seq

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func syncConfig(seq *evo.SequenceHandle, cfg []byte) {
	producer := seq.Task("write config")
	producer.Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{Path: "config.json", Contents: cfg})
	})

	consumer := seq.Task("read config")
	consumer.Define(func(ctx context.Context) error {
		_, err := os.ReadFile("config.json")
		return err
	})
}
