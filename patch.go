package evo

import (
	"context"

	"github.com/zachbornheimer/evident-output/internal/change"
)

// Patch derives the desired file states a unified text diff describes and
// commits each through File; see change.Patch and change.Files.
func Patch(ctx context.Context, diff []byte) error {
	set, err := derivePatch(ctx, diff)
	if err != nil {
		return err
	}
	return files(ctx, set)
}

// derivePatch derives the desired file states without committing them.
func derivePatch(ctx context.Context, diff []byte) (fileSet, error) {
	return change.Patch(ctx, diff)
}
