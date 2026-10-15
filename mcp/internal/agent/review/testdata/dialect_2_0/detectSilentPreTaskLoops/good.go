package fixture

import (
	"context"
	"io/fs"
	"path/filepath"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// run declares the Task first and walks the roots inside its Define.
func run(ctx context.Context, r *evo.Run, roots []string) error {
	r.Task("purge stale worktrees").Define(func(ctx context.Context) error {
		var stale []string
		for _, root := range roots {
			if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
				stale = append(stale, path)
				return nil
			}); err != nil {
				return evo.Fail("cannot walk " + root)
			}
		}
		return purge(stale)
	})
	return nil
}

func purge(paths []string) error { return nil }
