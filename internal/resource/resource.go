// Package resource coordinates concurrent access to shared state (spec
// ZYS-840): a filesystem subtree or a named logical resource, claimed for
// reading or writing. Read claims share; any overlapping pair that includes
// a write excludes. A holder may own at most one resource at a time, so
// deadlock is impossible by construction rather than by lock ordering.
//
// Nothing here is a public lock/unlock API. Callers get ownership only for
// the duration of a callback (Registry.HoldResource), which makes leaking it on
// error, panic, or cancellation inexpressible.
package resource

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	sysfs "github.com/zachbornheimer/evident-output/internal/fs"
)

// Resource names one unit of shared state. It is sealed: the unexported
// marker method means only FS and Logical can produce one.
type Resource interface {
	resource()
}

// fsResource is FS's Resource: a filesystem path, resolved at acquisition
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

// Label is r as the caller named it, with no namespace prefix: the path
// passed to FS or the name passed to Logical. It is what a person reads
// when a claim on r has to wait.
func Label(r Resource) string {
	switch r := r.(type) {
	case fsResource:
		return r.path
	case logicalResource:
		return strings.TrimSpace(r.name)
	default:
		return fmt.Sprint(r)
	}
}

// FS names the filesystem path path. A relative path resolves against the
// acquiring Run's workspace; symlinked ancestors resolve to their targets
// so an alias shares identity with the real path. Construction performs no
// I/O.
func FS(path string) Resource { return fsResource{path: path} }

// Logical names non-filesystem shared state. Names match exactly after
// surrounding whitespace is trimmed; they are not hierarchical.
func Logical(name string) Resource { return logicalResource{name: name} }

// ErrInvalid is returned when a Resource cannot name any state: an empty
// path or name, or a relative path with no absolute workspace to anchor it.
var ErrInvalid = errors.New("evo: invalid resource")

// space separates the filesystem and logical namespaces: a logical name
// never overlaps a path, however the strings compare.
type space string

const (
	spaceFS      space = "fs"
	spaceLogical space = "logical"
)

// Key is a Resource's resolved, canonical identity. Two claims coordinate
// only through their Keys, so every alias of the same state must resolve
// to the same Key.
type Key struct {
	space space
	name  string
}

// String renders the key as "fs:/abs/path" or "logical:name".
func (k Key) String() string { return string(k.space) + ":" + k.name }

// overlaps reports whether claims on k and o touch shared state: equal
// keys, or (filesystem only) one path is an ancestor directory of the other.
func (k Key) overlaps(o Key) bool {
	if k.space != o.space {
		return false
	}
	if k.name == o.name {
		return true
	}
	return k.space == spaceFS && (isAncestor(k.name, o.name) || isAncestor(o.name, k.name))
}

// isAncestor reports whether canonical path dir strictly contains path.
// The separator check keeps "/repo/a" from claiming "/repo/ab".
func isAncestor(dir, path string) bool {
	if !strings.HasSuffix(dir, string(filepath.Separator)) {
		dir += string(filepath.Separator)
	}
	return strings.HasPrefix(path, dir)
}

// resolver canonicalizes Resources into Keys, reading symlinks through
// evalSymlinks (facade rule). Every claim in the process resolves through
// the real disk, because a Key names shared state that every Output must
// agree on; a test builds its own resolver to inject a failure.
type resolver struct {
	evalSymlinks func(path string) (string, error)
}

// diskResolver is the resolver every claim uses.
var diskResolver = resolver{evalSymlinks: sysfs.EvalSymlinks}

// Resolve returns r's canonical Key. workspace anchors a relative FS path
// and must be absolute when one is given.
func Resolve(r Resource, workspace string) (Key, error) {
	return diskResolver.resolve(r, workspace)
}

func (rv resolver) resolve(r Resource, workspace string) (Key, error) {
	switch r := r.(type) {
	case fsResource:
		return rv.resolveFS(r.path, workspace)
	case logicalResource:
		name := strings.TrimSpace(r.name)
		if name == "" {
			return Key{}, fmt.Errorf("%w: logical resource name is empty", ErrInvalid)
		}
		return Key{space: spaceLogical, name: name}, nil
	default:
		return Key{}, fmt.Errorf("%w: %v is not a resource", ErrInvalid, r)
	}
}

func (rv resolver) resolveFS(path, workspace string) (Key, error) {
	if path == "" {
		return Key{}, fmt.Errorf("%w: filesystem resource path is empty", ErrInvalid)
	}
	if !filepath.IsAbs(path) {
		if !filepath.IsAbs(workspace) {
			return Key{}, fmt.Errorf("%w: relative path %q needs an absolute workspace, got %q", ErrInvalid, path, workspace)
		}
		path = filepath.Join(workspace, path)
	}
	canonical, err := rv.canonicalPath(filepath.Clean(path))
	if err != nil {
		return Key{}, fmt.Errorf("evo: resolve filesystem resource %q: %w", path, err)
	}
	return Key{space: spaceFS, name: canonical}, nil
}

// canonicalPath resolves every symlink along the longest existing prefix
// of the absolute, clean path, then re-appends the not-yet-existing tail.
// A path that does not exist yet (a File about to be created) therefore
// still shares identity with its real parent directory.
func (rv resolver) canonicalPath(path string) (string, error) {
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
