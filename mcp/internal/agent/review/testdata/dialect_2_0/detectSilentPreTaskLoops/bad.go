package fixture

import (
	"context"
	"io/fs"
	"path/filepath"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// run walks every root before it declares a Task, so the screen stays blank.
func run(ctx context.Context, r *evo.Run, roots []string) error {
	var stale []string
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			stale = append(stale, path)
			return nil
		})
	}
	r.Task("purge stale worktrees").Define(func(ctx context.Context) error { return purge(stale) })
	return nil
}

func purge(paths []string) error { return nil }
