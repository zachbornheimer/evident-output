package checksum

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"sync"
)

// Strategy computes one regular file's digest: the leaf of every tree and
// the whole of File.Checksum. Content is the only strategy today. A
// stat-stamp or clone-identity strategy satisfies the same interface and
// may decide from info alone, falling back to Content when it cannot.
type Strategy interface {
	Leaf(ctx context.Context, src Source, path string, info fs.FileInfo) (Digest, error)
}

// Content digests a file by streaming its bytes through SHA-256. It never
// buffers a whole file.
type Content struct{}

// copyBufferSize is the streaming chunk Content reads per call.
const copyBufferSize = 64 << 10

// copyBuffers recycles Content's streaming buffers across leaves.
var copyBuffers = sync.Pool{New: func() any {
	buf := make([]byte, copyBufferSize)
	return &buf
}}

// Leaf implements Strategy.
func (Content) Leaf(ctx context.Context, src Source, path string, _ fs.FileInfo) (Digest, error) {
	r, err := src.Open(path)
	if err != nil {
		return Digest{}, fmt.Errorf("checksum: open %s: %w", path, err)
	}
	defer func() { _ = r.Close() }()
	h := sha256.New()
	buf := copyBuffers.Get().(*[]byte)
	defer copyBuffers.Put(buf)
	if _, err := io.CopyBuffer(h, stoppableReader{stopped: ctx.Err, r: r}, *buf); err != nil {
		return Digest{}, fmt.Errorf("checksum: read %s: %w", path, err)
	}
	var d Digest
	h.Sum(d[:0])
	return d, nil
}

// stoppableReader ends a long read as soon as stopped reports an error
// (the caller's context.Err), so a cancelled digest of a huge file returns
// promptly instead of after the last byte.
type stoppableReader struct {
	stopped func() error
	r       io.Reader
}

func (s stoppableReader) Read(p []byte) (int, error) {
	if err := s.stopped(); err != nil {
		return 0, err
	}
	return s.r.Read(p)
}
