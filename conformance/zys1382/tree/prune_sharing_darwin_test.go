//go:build darwin

package tree_test

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/unix"

	evo "github.com/zachbornheimer/evident-output"
)

// pruneSharedRel is a file big enough that the filesystem stores it in
// its own extent rather than inline, so its physical address is a fair
// sharing probe.
const (
	pruneSharedRel  = "lib/shared.bin"
	pruneSharedSize = 1 << 20
)

// log2physDevOffset is where l2p_devoffset sits in the packed
// struct log2phys (flags uint32, contigbytes off_t, devoffset off_t).
const log2physDevOffset = 12

// prunePhysicalOffset is the device offset holding the first byte of the
// file at path (F_LOG2PHYS_EXT): two files sharing that block are clones
// of one another; a private copy has its own.
func prunePhysicalOffset(t *testing.T, path string) int64 {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var l2p unix.Log2phys_t
	raw := (*[unsafe.Sizeof(l2p)]byte)(unsafe.Pointer(&l2p))
	binary.NativeEndian.PutUint64(raw[4:], 1) // l2p_contigbytes: query one byte
	_, err = unix.FcntlInt(f.Fd(), unix.F_LOG2PHYS_EXT, int(uintptr(unsafe.Pointer(&l2p))))
	runtime.KeepAlive(&l2p)
	if err != nil {
		t.Skipf("F_LOG2PHYS_EXT %s: %v (no physical-extent probe on this filesystem)", path, err)
	}
	return int64(binary.NativeEndian.Uint64(raw[log2physDevOffset:]))
}

// addSharedFile writes a durable pruneSharedSize file into the tree at
// root, so its blocks are allocated before anything clones them.
func addSharedFile(t *testing.T, root string) {
	t.Helper()
	path := filepath.Join(root, pruneSharedRel)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(strings.Repeat("evo", pruneSharedSize/3)); err != nil {
		t.Fatal(err)
	}
	if err := f.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestPrune_RepublishedTreeSharesBlocksWithTheCanonicalTree(t *testing.T) {
	canonical := cloneSource(t)
	addSharedFile(t, canonical)
	f := pruneSwapFixture{canonical: canonical, parent: t.TempDir()}
	f.dest = filepath.Join(f.parent, "pkg")
	pruneByteCopy(t, canonical, f.dest)
	addSharedFile(t, f.dest)
	f.expected = pruneChecksum(t, f.dest)

	source := prunePhysicalOffset(t, filepath.Join(canonical, pruneSharedRel))
	if private := prunePhysicalOffset(t, filepath.Join(f.dest, pruneSharedRel)); private == source {
		t.Fatalf("fixture: the private copy already shares block %d with the canonical tree", source)
	}
	if got, err := f.swap(t, evo.Republish()); err != nil || !got.Published {
		t.Fatalf("ReplaceTree(Republish) = %+v, %v; want Published", got, err)
	}
	if after := prunePhysicalOffset(t, filepath.Join(f.dest, pruneSharedRel)); after != source {
		t.Fatalf("republished file is at block %d, canonical at %d; want one shared block", after, source)
	}
	pruneCloneIsIndependent(t, canonical, f.dest)
}

func TestPrune_WritableCloneSharesBlocksWithTheSealedSource(t *testing.T) {
	src := cloneSource(t)
	addSharedFile(t, src)
	pruneChmodTree(t, src, sealedDirMode, sealedFileMode, sealedExecMode)
	t.Cleanup(func() { pruneChmodTree(t, src, writableDirMode, writableFileMode, writableExecMode) })
	dst := filepath.Join(t.TempDir(), "pkg")
	writeWritableClone(t, src, dst)

	source := prunePhysicalOffset(t, filepath.Join(src, pruneSharedRel))
	if got := prunePhysicalOffset(t, filepath.Join(dst, pruneSharedRel)); got != source {
		t.Fatalf("writable clone file is at block %d, source at %d; making it writable must not copy data", got, source)
	}
	pruneCloneIsIndependent(t, src, dst)
}

// pruneCloneIsIndependent proves a write through the clone leaves the
// source's bytes alone.
func pruneCloneIsIndependent(t *testing.T, src, clone string) {
	t.Helper()
	before := pruneChecksum(t, src)
	if err := os.WriteFile(filepath.Join(clone, pruneSharedRel), []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	if after := pruneChecksum(t, src); after != before {
		t.Fatalf("writing the clone changed the source: %s -> %s", before, after)
	}
}
