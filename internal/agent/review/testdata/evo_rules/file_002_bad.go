// Fixture: EVO-FILE-002 must fire. Copying Path/Contents into a new
// FileSpec drops Patch Basis.
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
		if err := evo.File(ctx, evo.FileSpec{Path: spec.Path, Contents: spec.Contents}); err != nil {
			return err
		}
	}
	return nil
}
