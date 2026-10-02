// Package find discovers regular files by exact base name beneath a root.
// The walk is bounded-parallel, cancellable, never opens file content,
// never follows symlinks below the root, and takes no lock.
package find

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"sync"

	"github.com/zachbornheimer/evident-output/internal/publish"
)

// DefaultWorkers bounds concurrent directory reads, and so the descriptors
// and goroutines one search holds.
const DefaultWorkers = 16

// ErrNotDirectory is a root that is not a directory.
var ErrNotDirectory = errors.New("find: root is not a directory")

// Source is the filesystem a search reads: metadata and listings only.
type Source interface {
	Lstat(path string) (fs.FileInfo, error)
	ReadDir(path string) ([]fs.DirEntry, error)
}

// Walker searches a Source.
type Walker struct {
	Source  Source
	Workers int
}

// Search returns the paths, under display, of every regular file below root
// whose base name is in names, sorted. root is the directory actually read
// (already symlink-resolved); display is the prefix reported.
func (w Walker) Search(ctx context.Context, root, display string, names []string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := w.Source.Lstat(root)
	if err != nil {
		return nil, fmt.Errorf("find %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("find %s: %w", root, ErrNotDirectory)
	}
	s := &search{w: w, ctx: ctx, root: root, display: display, want: toSet(names), queue: []string{""}}
	s.cond = sync.NewCond(&s.mu)
	return s.run()
}

type search struct {
	w       Walker
	ctx     context.Context
	root    string
	display string
	want    map[string]struct{}

	mu      sync.Mutex
	cond    *sync.Cond
	queue   []string // relative directories awaiting a read
	active  int
	failed  error
	matches []string
}

func (s *search) run() ([]string, error) {
	workers := s.w.Workers
	if workers <= 0 {
		workers = DefaultWorkers
	}
	stop := context.AfterFunc(s.ctx, func() {
		s.mu.Lock()
		s.cond.Broadcast()
		s.mu.Unlock()
	})
	defer stop()
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() { ; s.work() })
	}
	wg.Wait()
	if s.failed != nil {
		return nil, s.failed
	}
	if err := s.ctx.Err(); err != nil {
		return nil, err
	}
	slices.Sort(s.matches)
	return s.matches, nil
}

func (s *search) next() (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for {
		if s.failed != nil || s.ctx.Err() != nil {
			return "", false
		}
		if n := len(s.queue); n > 0 {
			dir := s.queue[n-1]
			s.queue = s.queue[:n-1]
			s.active++
			return dir, true
		}
		if s.active == 0 {
			s.cond.Broadcast()
			return "", false
		}
		s.cond.Wait()
	}
}

func (s *search) work() {
	for {
		dir, ok := s.next()
		if !ok {
			return
		}
		subdirs, found, err := s.read(dir)
		s.mu.Lock()
		s.active--
		if err != nil && s.failed == nil {
			s.failed = err
		}
		s.queue = append(s.queue, subdirs...)
		s.matches = append(s.matches, found...)
		s.cond.Broadcast()
		s.mu.Unlock()
	}
}

// read lists one directory. A directory removed mid-walk is not an error.
func (s *search) read(rel string) (subdirs, found []string, err error) {
	entries, err := s.w.Source.ReadDir(filepath.Join(s.root, rel))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) && rel != "" {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("find: read directory %s: %w", filepath.Join(s.display, rel), err)
	}
	for _, e := range entries {
		name := e.Name()
		if publish.IsStaging(name) {
			continue
		}
		switch typ := e.Type(); {
		case typ.IsDir():
			subdirs = append(subdirs, filepath.Join(rel, name))
		case typ.IsRegular():
			if _, hit := s.want[name]; hit {
				found = append(found, filepath.Join(s.display, rel, name))
			}
		}
	}
	return subdirs, found, nil
}

func toSet(names []string) map[string]struct{} {
	set := make(map[string]struct{}, len(names))
	for _, n := range names {
		if n != "" {
			set[n] = struct{}{}
		}
	}
	return set
}
