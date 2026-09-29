// Fixture: EVO-DAG-003 must fire. "producer" establishes config.json via
// evo.File; "consumer" reads the same literal path, but nothing orders
// them — first-run scheduling gives no guarantee producer already ran.
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
	consumer.Define(func(ctx context.Context) error {
		_, err := os.ReadFile("config.json")
		return err
	})
}
