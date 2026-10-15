// Producer then consumer under one Sequence is already ordered.
package dag003

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func syncConfig(seq *evo.Sequence, cfg []byte) {
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
