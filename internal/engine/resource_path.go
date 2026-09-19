package engine

import (
	"path/filepath"

	"github.com/zachbornheimer/evident-output/internal/fingerprint"
)

// absPath and evalSymlinks are the facades canonicalFilePath uses instead
// of filepath.Abs/EvalSymlinks directly (facade rule).
var (
	absPath      = filepath.Abs
	evalSymlinks = filepath.EvalSymlinks
)

// canonicalFilePath is the process-local identity for a resource hold:
// absolute, cleaned, and symlink-resolved. A missing path resolves its
// existing ancestor and joins the missing tail, so aliases of the same
// file conflict even before the file exists.
func canonicalFilePath(path string) (string, error) {
	abs, err := absPath(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	resolved, err := evalSymlinks(abs)
	if err == nil {
		return resolved, nil
	}
	var missing []string
	current := abs
	for {
		parent := filepath.Dir(current)
		if parent == current {
			return abs, nil
		}
		missing = append(missing, filepath.Base(current))
		resolvedParent, parentErr := evalSymlinks(parent)
		if parentErr == nil {
			parts := make([]string, 0, 1+len(missing))
			parts = append(parts, resolvedParent)
			for i := len(missing) - 1; i >= 0; i-- {
				parts = append(parts, missing[i])
			}
			return filepath.Join(parts...), nil
		}
		current = parent
	}
}

func oneHold(path string, kind holdKind) (resourceHold, error) {
	canon, err := canonicalFilePath(path)
	if err != nil {
		return resourceHold{}, err
	}
	return resourceHold{path: canon, display: path, kind: kind}, nil
}

func (o *Output) fileHolds(spec FileSpec) ([]resourceHold, error) {
	if spec.Path == "" {
		return nil, nil
	}
	dest := o.resolveWorkspacePath(spec.Path)
	hold, err := oneHold(dest, holdWrite)
	if err != nil {
		return nil, err
	}
	return appendBasisHolds([]resourceHold{hold}, spec.Basis, o)
}

func (o *Output) execHolds(spec ExecSpec) ([]resourceHold, error) {
	dir := o.resolveWorkspacePath(spec.Dir)
	outputs := resolveExecOutputs(dir, spec.Outputs)
	holds := make([]resourceHold, 0, len(outputs)+len(spec.Basis))
	for _, out := range outputs {
		hold, err := oneHold(out, holdWrite)
		if err != nil {
			return nil, err
		}
		holds = append(holds, hold)
	}
	return appendBasisHolds(holds, spec.Basis, o)
}

func (o *Output) patchHolds(paths []string) ([]resourceHold, error) {
	holds := make([]resourceHold, 0, len(paths))
	for _, p := range paths {
		hold, err := oneHold(p, holdRead)
		if err != nil {
			return nil, err
		}
		holds = append(holds, hold)
	}
	return coalesceHolds(holds), nil
}

func appendBasisHolds(holds []resourceHold, basis []fingerprint.Fingerprint, o *Output) ([]resourceHold, error) {
	for _, b := range basis {
		p, ok := fingerprint.PathOf(b)
		if !ok {
			continue
		}
		hold, err := oneHold(o.resolveWorkspacePath(p), holdRead)
		if err != nil {
			return nil, err
		}
		holds = append(holds, hold)
	}
	return coalesceHolds(holds), nil
}
