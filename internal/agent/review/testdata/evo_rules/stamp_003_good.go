// Fixture: EVO-STAMP-003 must stay silent. Define + evo.File already
// resolves; there is no Done after it.
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
	return nil
}
