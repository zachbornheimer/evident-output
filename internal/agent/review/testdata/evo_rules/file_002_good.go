// Fixture: EVO-FILE-002 must stay silent. PatchResult specs pass through.
package file002

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

func apply(ctx context.Context, diff []byte, dir string) error {
	result, err := evo.Patch(ctx, evo.PatchSpec{Diff: diff, Dir: dir})
	if err != nil {
		return err
	}
	for _, spec := range result.Files {
		if err := evo.File(ctx, spec); err != nil {
			return err
		}
	}
	return nil
}
