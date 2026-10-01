package evo

import (
	"context"
	"io/fs"

	"github.com/zachbornheimer/evident-output/internal/engine"
)

// File is one regular file: where it lives and, optionally, what it should
// hold. A File literal performs no I/O; its methods do. Read, Checksum, and
// Equal observe; Write, Verify, and Remove establish or check state and
// must be called with a ctx from a Task's Define callback.
type File struct {
	// Path is required. A relative Path resolves against the Run's
	// workspace directory captured once at Run start.
	Path string
	// Content is the desired content (Bytes or Download). nil declares no
	// desired content; it never means "absent" (use Remove for that).
	Content FileContent
	// Mode is the desired permission bits. 0 means 0644 on create and
	// preserve on an existing file.
	Mode fs.FileMode
}

// Read returns the file's observed bytes. A missing path is fs.ErrNotExist.
func (f File) Read(ctx context.Context) ([]byte, error) { return nil, errNotImplemented }

// Write establishes Content (and Mode) at Path: preparation runs outside
// any lock, the result is published by atomic rename under a short
// destination lock that revalidates at commit, and the result is verified
// after commit. Already satisfied is a no-op. nil Content is
// ErrContentMissing.
func (f File) Write(ctx context.Context) error { return errNotImplemented }

// Verify reports whether Path holds Content: nil, or ErrVerifyMismatch.
func (f File) Verify(ctx context.Context) error { return errNotImplemented }

// Equal reports whether f and other hold the same bytes. Mode is ignored.
func (f File) Equal(ctx context.Context, other File) (bool, error) {
	return false, errNotImplemented
}

// FileContent is a File's desired content: Bytes or Download. Callers can
// hold one but not implement one.
type FileContent interface{ fileContent() }

// bytesContent is the FileContent Bytes returns.
type bytesContent struct{ data []byte }

func (bytesContent) fileContent() {}

// Bytes is literal file content. Bytes([]byte(nil)) and Bytes("") are an empty
// regular file, never an absent one.
func Bytes[T ~string | ~[]byte](v T) FileContent {
	return bytesContent{data: append([]byte(nil), v...)}
}

// File usage errors.
var (
	// ErrPathMissing is a File or Tree whose Path is empty.
	ErrPathMissing = engine.ErrPathMissing
	// ErrContentMissing is a Write whose Content is nil.
	ErrContentMissing = engine.ErrContentMissing
	// ErrVerifyMismatch is observed state that differs from declared Content.
	ErrVerifyMismatch = engine.ErrVerifyMismatch
	// ErrFilePathIsSymlink is an existing symlink occupying a File's Path.
	ErrFilePathIsSymlink = engine.ErrFilePathIsSymlink
	// ErrFilePathTypeMismatch is a File whose Path holds a non-regular file.
	ErrFilePathTypeMismatch = engine.ErrFilePathTypeMismatch
)

// errNotImplemented is the ZYS-1382 skeleton's answer from a primitive
// whose behavior has not landed yet.
var errNotImplemented = engine.ErrNotImplemented

// FileFS is the filesystem facade File and Tree operations read and write
// through (Config.FileFS).
type FileFS = engine.FileFS
