package download

import (
	"context"
	"io"
)

// Source is a Download's URL and Integrity, validated once. File.Write
// calls Fill while staging (outside any lock) and Matches to decide whether
// the destination is already current, so the integrity rules live here only.
type Source struct {
	url  string
	want Expected
}

// NewSource validates a Download before any I/O: an empty URL is
// ErrURLMissing, an empty or unparseable integrity is ErrIntegrity.
func NewSource(rawURL, integrity string) (Source, error) {
	if rawURL == "" {
		return Source{}, ErrURLMissing
	}
	want, err := Parse(integrity)
	if err != nil {
		return Source{}, err
	}
	return Source{url: rawURL, want: want}, nil
}

// Fill fetches the URL into w; nil means every byte arrived and matched.
func (s Source) Fill(ctx context.Context, w io.Writer) error {
	return Fetch(ctx, s.url, s.want, w)
}

// Matches reports whether r's bytes satisfy the integrity (no fetch).
func (s Source) Matches(r io.Reader) (bool, error) { return s.want.Matches(r) }
