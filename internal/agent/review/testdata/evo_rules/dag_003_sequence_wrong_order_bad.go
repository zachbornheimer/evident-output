// Fixture: EVO-DAG-003 must still fire. "consumer" is declared before
// "producer" in the same evo.Sequence — Sequence orders children in
// declaration order, so this is the reverse of what the read needs;
// membership in a Sequence alone is not enough, order matters.
package dag003seqwrong

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func syncConfig(seq *evo.SequenceHandle, cfg []byte) {
	consumer := seq.Task("read config")
	consumer.Define(func(ctx context.Context) error {
		_, err := os.ReadFile("config.json")
		return err
	})

	producer := seq.Task("write config")
	producer.Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{Path: "config.json", Contents: cfg})
	})
}
