package driver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	toolListSections     = "evident_output_list_sections"
	toolGetDocumentation = "evident_output_get_documentation"
	corpusCacheKey       = "docs-corpus.txt"
	cacheFileMode        = 0o644
	cacheDirMode         = 0o755
)

// BlobStore is the on-disk cache boundary.
type BlobStore interface {
	Get(key string) ([]byte, bool, error)
	Put(key string, data []byte) error
}

// DirBlobStore keeps blobs as files under Dir.
type DirBlobStore struct{ Dir string }

// Get reads a blob; a missing file is a miss, not an error.
func (s DirBlobStore) Get(key string) ([]byte, bool, error) {
	data, err := os.ReadFile(filepath.Join(s.Dir, key))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read cache entry %s in %s: %w", key, s.Dir, err)
	}
	return data, true, nil
}

// Put writes a blob, creating Dir when needed.
func (s DirBlobStore) Put(key string, data []byte) error {
	if err := os.MkdirAll(s.Dir, cacheDirMode); err != nil {
		return fmt.Errorf("create cache directory %s: %w", s.Dir, err)
	}
	if err := os.WriteFile(filepath.Join(s.Dir, key), data, cacheFileMode); err != nil {
		return fmt.Errorf("write cache entry %s in %s: %w", key, s.Dir, err)
	}
	return nil
}

// LoadCorpus returns the full docs text: from the cache when present, else
// fetched once through list_sections and get_documentation and then cached.
func LoadCorpus(ctx context.Context, server ToolServer, cache BlobStore) (string, error) {
	if cached, ok, err := cache.Get(corpusCacheKey); err != nil {
		return "", err
	} else if ok {
		return string(cached), nil
	}
	corpus, err := fetchCorpus(ctx, server)
	if err != nil {
		return "", err
	}
	if err := cache.Put(corpusCacheKey, []byte(corpus)); err != nil {
		return "", err
	}
	return corpus, nil
}

func fetchCorpus(ctx context.Context, server ToolServer) (string, error) {
	listing, err := server.Call(ctx, toolListSections, nil)
	if err != nil {
		return "", fmt.Errorf("fetch docs corpus: %w", err)
	}
	var index struct {
		Sections []struct {
			ID string `json:"id"`
		} `json:"sections"`
	}
	if err := json.Unmarshal(listing.Structured, &index); err != nil {
		return "", fmt.Errorf("parse %s result: %w", toolListSections, err)
	}
	ids := make([]string, 0, len(index.Sections))
	for _, section := range index.Sections {
		ids = append(ids, section.ID)
	}
	args, err := json.Marshal(map[string]any{"ids": ids})
	if err != nil {
		return "", fmt.Errorf("marshal section ids: %w", err)
	}
	docs, err := server.Call(ctx, toolGetDocumentation, args)
	if err != nil {
		return "", fmt.Errorf("fetch docs corpus: %w", err)
	}
	var bodies struct {
		Sections []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
			Body  string `json:"body"`
		} `json:"sections"`
	}
	if err := json.Unmarshal(docs.Structured, &bodies); err != nil {
		return "", fmt.Errorf("parse %s result: %w", toolGetDocumentation, err)
	}
	var corpus strings.Builder
	for _, section := range bodies.Sections {
		fmt.Fprintf(&corpus, "# %s (%s)\n\n%s\n\n", section.Title, section.ID, section.Body)
	}
	return corpus.String(), nil
}
