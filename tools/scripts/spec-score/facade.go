package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	filePerm = 0o644
	dirPerm  = 0o755
)

// FileSystem is the file boundary of the scorer; tests substitute a fake.
type FileSystem interface {
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte) error
	// Glob returns the paths matching pattern, sorted.
	Glob(pattern string) ([]string, error)
	// WalkFiles returns every file path under root ending in suffix. A
	// missing root yields no paths and no error.
	WalkFiles(root, suffix string) ([]string, error)
}

type osFileSystem struct{}

func (osFileSystem) ReadFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return data, nil
}

func (osFileSystem) WriteFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return fmt.Errorf("create directory for %s: %w", path, err)
	}
	if err := os.WriteFile(path, data, filePerm); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func (osFileSystem) Glob(pattern string) ([]string, error) {
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob %s: %w", pattern, err)
	}
	return paths, nil
}

func (osFileSystem) WalkFiles(root, suffix string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) && path == root {
				return filepath.SkipAll
			}
			return walkErr
		}
		if !d.IsDir() && strings.HasSuffix(path, suffix) {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", root, err)
	}
	return paths, nil
}
