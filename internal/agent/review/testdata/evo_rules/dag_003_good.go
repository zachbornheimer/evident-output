// Fixture: EVO-DAG-003 must stay silent. "consumer" declares
// consumer.After(producer), so the producer/consumer relationship has an
// explicit first-run scheduler edge.
package dag003

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func syncConfig(group *evo.GroupHandle, cfg []byte) {
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
