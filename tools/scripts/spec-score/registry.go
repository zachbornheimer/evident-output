package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

const (
	registryGlob  = "conformance/spec/req/*.json"
	ratchetPath   = "conformance/spec/ratchet.json"
	contractPath  = "conformance/spec/contract-1.x.md"
	contractTests = "conformance/contract"
)

// loadRegistry reads every registry file under root, keeping file order.
func loadRegistry(fsys FileSystem, root string) ([]Entry, error) {
	files, err := fsys.Glob(filepath.Join(root, registryGlob))
	if err != nil {
		return nil, fmt.Errorf("list registry files under %s: %w", root, err)
	}
	var entries []Entry
	for _, file := range files {
		data, err := fsys.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("load registry: %w", err)
		}
		var batch []Entry
		if err := json.Unmarshal(data, &batch); err != nil {
			return nil, fmt.Errorf("parse registry file %s: %w", file, err)
		}
		entries = append(entries, batch...)
	}
	return entries, nil
}
