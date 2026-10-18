// The consumer is After the producer.
package dag003

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func syncConfig(group *evo.Group, cfg []byte) {
	producer := group.Task("write config")
	producer.Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{Path: "config.json", Contents: cfg})
	})

	consumer := group.Task("read config")
	consumer.After(producer)
	consumer.Define(func(ctx context.Context) error {
		_, err := os.ReadFile("config.json")
		return err
	})
}
