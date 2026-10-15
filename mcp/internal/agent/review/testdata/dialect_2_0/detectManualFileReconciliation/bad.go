package fixture

import (
	"context"
	"fmt"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// declareConfig hand-rolls the write and chmod that FileSpec{Contents, Mode} covers.
func declareConfig(r *evo.Run, path string, data []byte) {
	r.Task("write config").Define(func(context.Context) error {
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return fmt.Errorf("write config %s: %w", path, err)
		}
		if err := os.Chmod(path, 0o644); err != nil {
			return fmt.Errorf("chmod config %s: %w", path, err)
		}
		return nil
	})
}
