// Package extract unpacks an archive (tar.gz, tar, zip; detected from
// content) into an empty staging directory, refusing every entry that
// could escape it. It never publishes: the caller stages a tree, runs
// Unpack as the fill, and commits only when Unpack succeeds.
package extract

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// ErrMalformed is an archive that is empty, unreadable, or truncated.
var ErrMalformed = errors.New("extract: malformed archive")

// ErrUnsafeEntry is an entry that would escape the destination, or is not
// a regular file, directory, or contained link.
var ErrUnsafeEntry = errors.New("extract: unsafe archive entry")

const (
	gzipMagic     = "\x1f\x8b"
	zipMagic      = "PK\x03\x04"
	zipEmptyMagic = "PK\x05\x06"
	sniffLen      = 4
)

// Unpack extracts the archive at archivePath into dest, which must be an
// existing empty directory. root, when set, is a leading path component
// every entry must sit under and is removed. The error wraps ErrMalformed,
// ErrUnsafeEntry, the context's error, or an operating-system error
// (a missing archive satisfies fs.ErrNotExist).
func Unpack(ctx context.Context, archivePath, root, dest string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("extract %s: %w", archivePath, err)
	}
	defer func() { _ = f.Close() }() // read-only: nothing to flush
	rd, err := newReader(f)
	if err != nil {
		return fmt.Errorf("extract %s: %w", archivePath, err)
	}
	w, err := newWriter(dest, root)
	if err != nil {
		return fmt.Errorf("extract %s: %w", archivePath, err)
	}
	if err := rd.each(ctx, w); err != nil {
		return fmt.Errorf("extract %s: %w", archivePath, err)
	}
	if w.entries == 0 {
		return fmt.Errorf("extract %s: %w: no entries", archivePath, ErrMalformed)
	}
	return nil
}

// walker feeds every archive member to a writer.
type walker interface {
	each(ctx context.Context, w *writer) error
}

// newReader picks the format from the first bytes of f.
func newReader(f *os.File) (walker, error) {
	head := make([]byte, sniffLen)
	n, err := io.ReadFull(f, head)
	if n == 0 {
		return nil, fmt.Errorf("%w: empty file", ErrMalformed)
	}
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, err
	}
	head = head[:n]
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	switch {
	case bytes.HasPrefix(head, []byte(gzipMagic)):
		return tarReader{src: f, gzipped: true}, nil
	case bytes.HasPrefix(head, []byte(zipMagic)), bytes.HasPrefix(head, []byte(zipEmptyMagic)):
		return zipReader{f: f}, nil
	}
	return tarReader{src: f}, nil
}

type tarReader struct {
	src     io.Reader
	gzipped bool
}

func (t tarReader) each(ctx context.Context, w *writer) error {
	src := &sourceReader{r: bufio.NewReader(t.src)}
	var stream io.Reader = src
	if t.gzipped {
		gz, err := gzip.NewReader(src)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrMalformed, err)
		}
		defer func() { _ = gz.Close() }() // read-only: nothing to flush
		stream = &sourceReader{r: gz}
	}
	tr := tar.NewReader(stream)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return malformed(ctx, err)
		}
		if err := w.addTar(ctx, hdr, tr); err != nil {
			return err
		}
	}
}

type zipReader struct{ f *os.File }

func (z zipReader) each(ctx context.Context, w *writer) error {
	info, err := z.f.Stat()
	if err != nil {
		return err
	}
	zr, err := zip.NewReader(z.f, info.Size())
	if err != nil {
		return fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	for _, e := range zr.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := w.addZip(ctx, e); err != nil {
			return err
		}
	}
	return nil
}

// sourceReader marks errors that came from reading the archive (as
// opposed to writing the destination), so they classify as malformed.
type sourceReader struct{ r io.Reader }

type sourceError struct{ err error }

func (e sourceError) Error() string { return e.err.Error() }
func (e sourceError) Unwrap() error { return e.err }

func (s *sourceReader) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		err = sourceError{err}
	}
	return n, err
}

// malformed classifies an archive-read failure: cancellation passes
// through, everything else is a malformed archive.
func malformed(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return fmt.Errorf("%w: %v", ErrMalformed, err)
}

// classifyCopy maps an io.Copy failure to its cause.
func classifyCopy(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	var se sourceError
	if errors.As(err, &se) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, zip.ErrChecksum) {
		return fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	return err
}

// ctxReader stops a long copy the moment ctx ends.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

func unsafe(name, why string) error {
	return fmt.Errorf("%w: %q: %s", ErrUnsafeEntry, name, strings.TrimSpace(why))
}
