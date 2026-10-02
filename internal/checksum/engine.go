package checksum

import (
	"context"
	"fmt"
	"io/fs"
	"runtime"
)

// Engine computes file and tree digests. The zero Engine reads the real
// filesystem with the Content strategy. It caches nothing: every call
// observes the filesystem as it is now.
type Engine struct {
	// Source is where bytes and listings come from. nil is OS.
	Source Source
	// Strategy digests one regular file. nil is Content.
	Strategy Strategy
	// Parallelism bounds concurrent leaf digests inside one Tree call.
	// 0 or less is runtime.GOMAXPROCS(0).
	Parallelism int
	// Observe, when set, sees every leaf digested and every directory
	// listed, from whichever goroutine did the work. It proves routing in
	// tests and feeds diagnostics; it must be safe for concurrent use.
	Observe func(Event)
}

// Op is what an Event reports.
type Op uint8

const (
	// OpLeaf is one regular file digested through the Strategy.
	OpLeaf Op = iota + 1
	// OpList is one directory listed through the Source.
	OpList
)

// Event is one unit of engine work on Path.
type Event struct {
	Op   Op
	Path string
}

// File returns the digest of the regular file at path. A symlink is
// ErrSymlink and any other non-regular file is ErrNotRegular; neither is
// ever opened.
func (e Engine) File(ctx context.Context, path string) (Digest, error) {
	if err := ctx.Err(); err != nil {
		return Digest{}, err
	}
	info, err := e.source().Lstat(path)
	if err != nil {
		return Digest{}, err
	}
	switch {
	case info.Mode()&fs.ModeSymlink != 0:
		return Digest{}, fmt.Errorf("%w: %s", ErrSymlink, path)
	case !info.Mode().IsRegular():
		return Digest{}, fmt.Errorf("%w: %s is %v", ErrNotRegular, path, info.Mode().Type())
	}
	return e.leaf(ctx, path, info)
}

// Tree returns the Merkle digest of the directory at root, leaving out
// every entry exclude matches. root must be a directory itself (a caller
// that accepts a symlinked root resolves it first); symlinks below root are
// digested as links, never followed.
func (e Engine) Tree(ctx context.Context, root string, exclude Exclusion) (Digest, error) {
	if err := ctx.Err(); err != nil {
		return Digest{}, err
	}
	info, err := e.source().Lstat(root)
	if err != nil {
		return Digest{}, err
	}
	if !info.IsDir() {
		return Digest{}, fmt.Errorf("%w: %s is %v", ErrNotDirectory, root, info.Mode().Type())
	}
	return e.walkTree(ctx, root, exclude)
}

// leaf is the one digest of one regular file, shared by File and every
// tree leaf.
func (e Engine) leaf(ctx context.Context, path string, info fs.FileInfo) (Digest, error) {
	e.observe(OpLeaf, path)
	return e.strategy().Leaf(ctx, e.source(), path, info)
}

func (e Engine) source() Source {
	if e.Source == nil {
		return OS{}
	}
	return e.Source
}

func (e Engine) strategy() Strategy {
	if e.Strategy == nil {
		return Content{}
	}
	return e.Strategy
}

func (e Engine) parallelism() int {
	if e.Parallelism <= 0 {
		return runtime.GOMAXPROCS(0)
	}
	return e.Parallelism
}

func (e Engine) observe(op Op, path string) {
	if e.Observe != nil {
		e.Observe(Event{Op: op, Path: path})
	}
}
