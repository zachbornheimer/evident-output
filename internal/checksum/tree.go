package checksum

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
)

// treeDomain prefixes every directory digest so a directory can never
// collide with a file whose bytes spell the same framing. v2 frames
// executable regular files apart from plain ones; the version bump keeps a
// stored v1 digest from ever equalling a v2 digest of a chmod'd tree.
const treeDomain = "evo.tree.v2\x00"

// linkDomain prefixes a symlink's target text for the same reason.
const linkDomain = "evo.link.v1\x00"

// kind is an entry's type tag inside a directory digest.
type kind byte

const (
	kindDir     kind = 'd'
	kindFile    kind = 'f'
	kindExec    kind = 'x'
	kindSymlink kind = 'l'
	kindFIFO    kind = 'p'
	kindSocket  kind = 's'
	kindDevice  kind = 'b'
	kindChar    kind = 'c'
	kindOther   kind = '?'
)

// execBits are the permission bits that make a regular file executable.
// Any one of them makes it kindExec: identity tracks "runs", not who may.
const execBits fs.FileMode = 0o111

// kindOf tags a directory entry by its type bits. Only directories,
// regular files, and symlinks carry content; the rest are structure. A
// regular file's executable bit is known only from its full mode, so the
// leaf digest upgrades kindFile to kindExec (see leafKind).
func kindOf(mode fs.FileMode) kind {
	switch t := mode.Type(); {
	case t == 0:
		return kindFile
	case t&fs.ModeDir != 0:
		return kindDir
	case t&fs.ModeSymlink != 0:
		return kindSymlink
	case t&fs.ModeNamedPipe != 0:
		return kindFIFO
	case t&fs.ModeSocket != 0:
		return kindSocket
	case t&fs.ModeCharDevice != 0:
		return kindChar
	case t&fs.ModeDevice != 0:
		return kindDevice
	default:
		return kindOther
	}
}

// node is one entry of the walked tree; a directory's digest is computed
// from its children once every leaf digest has landed.
type node struct {
	name     string
	kind     kind
	digest   Digest
	children []*node
}

// treeWalk is one Tree call: the engine, what it leaves out, and the pool
// its leaf digests run on.
type treeWalk struct {
	engine  Engine
	exclude Exclusion
	leaves  *leafPool
}

// walkTree lists the tree sequentially (cheap), digests leaves in parallel
// (expensive), then combines bottom-up in sorted order so the result never
// depends on scheduling.
func (e Engine) walkTree(ctx context.Context, root string, exclude Exclusion) (Digest, error) {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	w := treeWalk{engine: e, exclude: exclude, leaves: newLeafPool(e.parallelism(), cancel)}
	top := &node{kind: kindDir}
	walkErr := w.list(ctx, root, "/", top)
	leafErr := w.leaves.wait()
	// The cause comes first: a leaf failure cancels the walk with itself as
	// the cause, so the walk's own "context canceled" never hides it.
	if failure := cmp.Or(context.Cause(ctx), leafErr, walkErr); failure != nil {
		return Digest{}, fmt.Errorf("checksum: tree %s: %w", root, failure)
	}
	return top.combine(), nil
}

// list fills dir's children from the directory at path, whose in-tree path
// is rel ("/" for the root, "/a/" below it).
func (w treeWalk) list(ctx context.Context, path, rel string, dir *node) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("checksum: list directory %s: %w", path, err)
	}
	w.engine.observe(OpList, path)
	entries, err := w.engine.source().ReadDir(path)
	if err != nil {
		return fmt.Errorf("checksum: list directory %s: %w", path, err)
	}
	slices.SortFunc(entries, func(a, b fs.DirEntry) int { return cmp.Compare(a.Name(), b.Name()) })
	for _, entry := range entries {
		child, err := w.entry(ctx, path, rel, entry)
		if err != nil {
			return fmt.Errorf("checksum: entry %s: %w", filepath.Join(path, entry.Name()), err)
		}
		if child != nil {
			dir.children = append(dir.children, child)
		}
	}
	return nil
}

// entry builds one child node, or nil when the exclusion drops it.
// Excluded entries are never read or descended into.
func (w treeWalk) entry(ctx context.Context, parent, rel string, entry fs.DirEntry) (*node, error) {
	child := &node{name: entry.Name(), kind: kindOf(entry.Type())}
	path := filepath.Join(parent, child.name)
	childRel := rel + child.name
	if child.kind == kindDir {
		childRel += "/"
	}
	if w.exclude.excludes(childRel) {
		return nil, nil
	}
	switch child.kind {
	case kindDir:
		return child, w.list(ctx, path, childRel, child)
	case kindFile:
		w.leaves.digest(ctx, child, func(ctx context.Context) (Digest, error) { return w.engine.leafAt(ctx, path, entry, child) })
	case kindSymlink:
		target, err := w.engine.source().Readlink(path)
		if err != nil {
			return nil, fmt.Errorf("checksum: read symlink %s: %w", path, err)
		}
		child.digest = sha256.Sum256([]byte(linkDomain + target))
	}
	return child, nil
}

// leafAt digests the regular file a directory listing named and records
// its kind from the full mode. It runs on a leaf goroutine; combine reads
// n.kind only after the pool's wait.
func (e Engine) leafAt(ctx context.Context, path string, entry fs.DirEntry, n *node) (Digest, error) {
	info, err := entry.Info()
	if err != nil {
		return Digest{}, fmt.Errorf("checksum: stat %s: %w", path, err)
	}
	n.kind = leafKind(info.Mode())
	return e.leaf(ctx, path, info)
}

// leafKind is a regular file's kind: kindExec when any execute bit is set.
func leafKind(mode fs.FileMode) kind {
	if mode&execBits != 0 {
		return kindExec
	}
	return kindFile
}

// combine is n's directory digest: the domain tag, then each child (already
// sorted by name) framed as kind, length-prefixed name, and child digest.
func (n *node) combine() Digest {
	h := sha256.New()
	_, _ = h.Write([]byte(treeDomain))
	var frame []byte
	for _, child := range n.children {
		if child.kind == kindDir {
			child.digest = child.combine()
		}
		frame = append(frame[:0], byte(child.kind))
		frame = binary.AppendUvarint(frame, uint64(len(child.name)))
		frame = append(frame, child.name...)
		frame = append(frame, child.digest[:]...)
		_, _ = h.Write(frame)
	}
	var d Digest
	h.Sum(d[:0])
	return d
}
