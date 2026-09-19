package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zachbornheimer/evident-output/internal/fingerprint"
)

// PatchSpec names a unified diff to derive FileSpecs from. Constructing it
// performs no I/O; Patch performs the operation and never writes.
type PatchSpec struct {
	// Diff is a unified diff. Patch never writes the workspace.
	Diff []byte
	// Dir is the directory relative paths in Diff resolve against. Empty
	// uses the Run workspace captured at start.
	Dir string
}

// PatchResult is the desired file state Patch derived in memory.
type PatchResult struct {
	Files []FileSpec
}

// Patch-specific usage errors. Callers match with errors.Is.
var (
	ErrPatchDelete    = errors.New("evo: Patch does not support deleting files")
	ErrPatchRename    = errors.New("evo: Patch does not support renaming files")
	ErrPatchBinary    = errors.New("evo: Patch does not support binary diffs")
	ErrPatchMalformed = errors.New("evo: Patch does not match the observed source")
)

// Patch identifies the source files a unified diff names, fingerprints
// them as Basis, and derives the desired resulting FileSpecs in memory.
// It mutates nothing on disk. ctx must come from a Task's Define callback.
func Patch(ctx context.Context, spec PatchSpec) (PatchResult, error) {
	scope, task, err := beginPublicResource(ctx)
	if err != nil {
		return PatchResult{}, err
	}
	defer scope.endPublicResource()
	return task.out.derivePatch(ctx, task, spec)
}

func (o *Output) derivePatch(ctx context.Context, task *TaskHandle, spec PatchSpec) (PatchResult, error) {
	if err := ctx.Err(); err != nil {
		return PatchResult{}, fmt.Errorf("evo: Patch: %w", err)
	}
	parsed, err := parseUnifiedDiff(spec.Diff)
	if err != nil {
		return PatchResult{}, err
	}
	dir := spec.Dir
	if dir == "" {
		dir = o.workspaceDirLocked()
	} else {
		dir = o.resolveWorkspacePath(dir)
	}

	var sourcePaths []string
	for _, f := range parsed {
		if unsup := f.unsupported(); unsup != nil {
			return PatchResult{}, unsup
		}
		if f.oldPath != "" && f.oldPath != "/dev/null" {
			sourcePaths = append(sourcePaths, filepath.Join(dir, f.oldPath))
		}
	}
	holds, err := o.patchHolds(sourcePaths)
	if err != nil {
		return PatchResult{}, err
	}
	drop, err := processResources.acquire(ctx, task, holds)
	if err != nil {
		return PatchResult{}, err
	}
	defer drop()

	files := make([]FileSpec, 0, len(parsed))
	fsys := o.fileFS()
	for _, f := range parsed {
		derived, deriveErr := o.derivePatchFile(ctx, fsys, dir, f)
		if deriveErr != nil {
			return PatchResult{}, deriveErr
		}
		files = append(files, derived)
	}
	return PatchResult{Files: files}, nil
}

func (o *Output) derivePatchFile(ctx context.Context, fsys FileFS, dir string, f parsedPatchFile) (FileSpec, error) {
	destRel := f.newPath
	if destRel == "" {
		destRel = f.oldPath
	}
	dest := filepath.Join(dir, destRel)
	if f.oldPath == "/dev/null" {
		contents, err := applyHunks(nil, f.hunks)
		if err != nil {
			return FileSpec{}, err
		}
		return FileSpec{Path: dest, Contents: contents, Mode: f.mode}, nil
	}

	srcPath := filepath.Join(dir, f.oldPath)
	src, err := fsys.ReadFile(srcPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return FileSpec{}, fmt.Errorf("evo: Patch read %q: %w", srcPath, err)
		}
		return FileSpec{}, fmt.Errorf("%w: %s", ErrPatchMalformed, srcPath)
	}
	fp := fingerprint.FSPath(srcPath)
	snap, err := fp.Fingerprint(ctx)
	if err != nil {
		return FileSpec{}, fmt.Errorf("evo: Patch Basis %q: %w", srcPath, err)
	}
	contents, err := applyHunks(src, f.hunks)
	if err != nil {
		return FileSpec{}, fmt.Errorf("%w: %s", err, srcPath)
	}
	return FileSpec{
		Path:       dest,
		Contents:   contents,
		Mode:       f.mode,
		Basis:      []fingerprint.Fingerprint{fp},
		patchBasis: []fingerprint.FingerprintValue{snap},
	}, nil
}
