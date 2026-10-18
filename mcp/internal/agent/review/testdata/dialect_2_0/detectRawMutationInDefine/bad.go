package fixture

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// declareConfig writes with os.WriteFile inside Define, which dry run cannot skip.
func declareConfig(r *evo.Run, path string, data []byte) {
	r.Task("write config").Define(func(context.Context) error {
		return os.WriteFile(path, data, 0o644)
	})
}
