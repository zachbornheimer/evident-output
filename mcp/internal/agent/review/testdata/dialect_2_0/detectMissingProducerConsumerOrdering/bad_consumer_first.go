// A Sequence orders by declaration, so a consumer declared first is unordered.
package dag003

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func syncConfig(seq *evo.Sequence, cfg []byte) {
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
