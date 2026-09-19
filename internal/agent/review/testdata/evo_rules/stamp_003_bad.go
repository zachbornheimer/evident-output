// Fixture: EVO-STAMP-003 must fire. Define already resolves from the
// callback; Done after it is double-resolution.
package stamp003

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

func writeConfig(ctx context.Context, out *evo.Output, path string, data []byte, basis []evo.Fingerprint) error {
	t := out.Task("write config")
	t.Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{Path: path, Contents: data, Mode: 0o644, Basis: basis})
	})
	t.Done()
	return nil
}
