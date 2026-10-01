package engine

import (
	"bytes"
	"context"
	"io"

	"github.com/zachbornheimer/evident-output/internal/download"
)

// DownloadSource is the FileSource for a Download. Validation errors (empty
// URL, bad Integrity) surface from Holds and Fill, both before any request.
func DownloadSource(url, integrity string) FileSource {
	src, err := download.NewSource(url, integrity)
	return downloadSource{src: src, err: err}
}

type downloadSource struct {
	src download.Source
	err error
}

func (s downloadSource) Fill(ctx context.Context, w io.Writer) error {
	if s.err != nil {
		return s.err
	}
	return s.src.Fill(ctx, w)
}

func (s downloadSource) Holds(ctx context.Context, path string) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	data, err := checksumScope(ctx).fileFSOrDefault().ReadFile(path)
	if err != nil {
		return false, err
	}
	return s.src.Matches(bytes.NewReader(data))
}
