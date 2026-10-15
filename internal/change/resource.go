// This file names the shared state concurrent changes coordinate on (spec
// ZYS-840): a filesystem subtree or a named logical resource, resolved to
// the canonical key two claims compare.

package change

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	sysfs "github.com/zachbornheimer/evident-output/v2/internal/fs"
)

// Resource names one unit of shared state. It is sealed: the unexported
// marker method means only FSResource and LogicalResource can produce one.
type Resource interface {
	resource()
}

// fsResource is FSResource's Resource: a filesystem path, resolved at acquisition
// against the acquiring Run's workspace.
type fsResource struct{ path string }

// logicalResource is Logical's Resource: a named, non-filesystem unit.
type logicalResource struct{ name string }

func (fsResource) resource()      {}
func (logicalResource) resource() {}

// String reports the resource as written by the caller, before resolution.
func (r fsResource) String() string { return string(spaceFS) + ":" + r.path }

// String reports the resource as written by the caller, before resolution.
func (r logicalResource) String() string { return string(spaceLogical) + ":" + r.name }

// ResourceLabel is r as the caller named it, with no namespace prefix: the path
// passed to FS or the name passed to Logical. It is what a person reads
// when a claim on r has to wait.
func ResourceLabel(r Resource) string {
	switch r := r.(type) {
	case fsResource:
		return r.path
	case logicalResource:
		return strings.TrimSpace(r.name)
	default:
		return fmt.Sprint(r)
	}
}

// FSResource names the filesystem path path. A relative path resolves against the
// acquiring Run's workspace; symlinked ancestors resolve to their targets
// so an alias shares identity with the real path. Construction performs no
// I/O.
func FSResource(path string) Resource { return fsResource{path: path} }

// LogicalResource names non-filesystem shared state. Names match exactly after
// surrounding whitespace is trimmed; they are not hierarchical.
func LogicalResource(name string) Resource { return logicalResource{name: name} }

// ErrInvalidResource is returned when a Resource cannot name any state: an empty
// path or name, or a relative path with no absolute workspace to anchor it.
var ErrInvalidResource = errors.New("evo: invalid resource")

// resourceSpace separates the filesystem and logical namespaces: a logical name
// never overlaps a path, however the strings compare.
type resourceSpace string

const (
	spaceFS      resourceSpace = "fs"
	spaceLogical resourceSpace = "logical"
)

// ResourceKey is a Resource's resolved, canonical identity. Two claims coordinate
// only through their Keys, so every alias of the same state must resolve
// to the same ResourceKey.
type ResourceKey struct {
	space resourceSpace
	name  string
}

// String renders the key as "fs:/abs/path" or "logical:name".
func (k ResourceKey) String() string { return string(k.space) + ":" + k.name }

// overlaps reports whether claims on k and o touch shared state: equal
// keys, or (filesystem only) one path is an ancestor directory of the other.
func (k ResourceKey) overlaps(o ResourceKey) bool {
	switch {
	case k.space != o.space:
		return false
	case k.space == spaceFS:
		return pathsOverlap(k.name, o.name)
	default:
		return k.name == o.name
	}
}

// pathsOverlap reports whether claims on canonical paths a and b exclude
// each other: the same path, or one an ancestor directory of the other.
func pathsOverlap(a, b string) bool {
	return a == b || isAncestor(a, b) || isAncestor(b, a)
}

// isAncestor reports whether canonical path dir strictly contains path.
// The separator check keeps "/repo/a" from claiming "/repo/ab".
func isAncestor(dir, path string) bool {
	if !strings.HasSuffix(dir, string(filepath.Separator)) {
		dir += string(filepath.Separator)
	}
	return strings.HasPrefix(path, dir)
}

// resourceResolver canonicalizes Resources into Keys, reading symlinks through
// evalSymlinks (facade rule). Every claim in the process resolves through
// the real disk, because a ResourceKey names shared state that every Output must
// agree on; a test builds its own resourceResolver to inject a failure.
type resourceResolver struct {
	evalSymlinks func(path string) (string, error)
}

// diskResourceResolver is the resolver every claim uses.
var diskResourceResolver = resourceResolver{evalSymlinks: sysfs.EvalSymlinks}

// ResolveResource returns r's canonical ResourceKey. workspace anchors a relative FS path
// and must be absolute when one is given.
func ResolveResource(r Resource, workspace string) (ResourceKey, error) {
	return diskResourceResolver.resolve(r, workspace)
}

func (rv resourceResolver) resolve(r Resource, workspace string) (ResourceKey, error) {
	switch r := r.(type) {
	case fsResource:
		return rv.resolveFS(r.path, workspace)
	case logicalResource:
		name := strings.TrimSpace(r.name)
		if name == "" {
			return ResourceKey{}, fmt.Errorf("%w: logical resource name is empty", ErrInvalidResource)
		}
		return ResourceKey{space: spaceLogical, name: name}, nil
	default:
		return ResourceKey{}, fmt.Errorf("%w: %v is not a resource", ErrInvalidResource, r)
	}
}

func (rv resourceResolver) resolveFS(path, workspace string) (ResourceKey, error) {
	if path == "" {
		return ResourceKey{}, fmt.Errorf("%w: filesystem resource path is empty", ErrInvalidResource)
	}
	if !filepath.IsAbs(path) {
		if !filepath.IsAbs(workspace) {
			return ResourceKey{}, fmt.Errorf("%w: relative path %q needs an absolute workspace, got %q", ErrInvalidResource, path, workspace)
		}
		path = filepath.Join(workspace, path)
	}
	canonical, err := rv.canonicalPath(filepath.Clean(path))
	if err != nil {
		return ResourceKey{}, fmt.Errorf("evo: resolve filesystem resource %q: %w", path, err)
	}
	return ResourceKey{space: spaceFS, name: canonical}, nil
}

// canonicalPath resolves every symlink along the longest existing prefix
// of the absolute, clean path, then re-appends the not-yet-existing tail.
// A path that does not exist yet (a File about to be created) therefore
// still shares identity with its real parent directory.
func (rv resourceResolver) canonicalPath(path string) (string, error) {
	existing, tail := path, ""
	for {
		resolved, err := rv.evalSymlinks(existing)
		if err == nil {
			return filepath.Join(resolved, tail), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return path, nil
		}
		tail = filepath.Join(filepath.Base(existing), tail)
		existing = parent
	}
}
