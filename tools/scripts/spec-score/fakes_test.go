package main

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
)

// fakeFS is an in-memory FileSystem keyed by slash paths.
type fakeFS struct{ files map[string]string }

func newFakeFS(files map[string]string) *fakeFS { return &fakeFS{files: files} }

func (f *fakeFS) ReadFile(p string) ([]byte, error) {
	data, ok := f.files[p]
	if !ok {
		return nil, fmt.Errorf("read %s: %w", p, fs.ErrNotExist)
	}
	return []byte(data), nil
}

func (f *fakeFS) WriteFile(p string, data []byte) error {
	f.files[p] = string(data)
	return nil
}

func (f *fakeFS) Glob(pattern string) ([]string, error) {
	var out []string
	for p := range f.files {
		if ok, _ := path.Match(pattern, p); ok {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (f *fakeFS) WalkFiles(root, suffix string) ([]string, error) {
	var out []string
	for p := range f.files {
		if strings.HasPrefix(p, root+"/") && strings.HasSuffix(p, suffix) {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out, nil
}

// fakeRunner returns canned event streams per package and records calls.
type fakeRunner struct {
	streams map[string]string
	errs    map[string]error
	calls   []string
}

func (r *fakeRunner) Run(_, pkg, _ string, names []string) ([]byte, error) {
	r.calls = append(r.calls, pkg+" "+strings.Join(names, ","))
	if err := r.errs[pkg]; err != nil {
		return nil, err
	}
	return []byte(r.streams[pkg]), nil
}

func event(action, test string) string {
	return fmt.Sprintf(`{"Action":%q,"Test":%q}`+"\n", action, test)
}
