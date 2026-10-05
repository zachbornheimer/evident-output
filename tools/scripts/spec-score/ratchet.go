package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
)

// loadRatchet reads the ratcheted IDs; a missing file is an empty ratchet.
func loadRatchet(fsys FileSystem, root string) ([]string, error) {
	path := filepath.Join(root, ratchetPath)
	data, err := fsys.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		return nil, fmt.Errorf("parse ratchet %s: %w", path, err)
	}
	return ids, nil
}

// gateRegressions returns the ratcheted IDs that are not passing.
func gateRegressions(ratchet []string, score Score) []string {
	var out []string
	for _, id := range ratchet {
		if score.IDs[id] != StatusPass {
			out = append(out, id)
		}
	}
	return out
}

// ratchetWithPassing returns ratchet plus every passing ID, sorted and
// deduplicated. It never drops an ID.
func ratchetWithPassing(ratchet []string, score Score) []string {
	all := append([]string{}, ratchet...)
	for id, status := range score.IDs {
		if status == StatusPass {
			all = append(all, id)
		}
	}
	return uniqueSorted(all)
}

func writeRatchet(fsys FileSystem, root string, ids []string) error {
	data, err := json.MarshalIndent(ids, "", "  ")
	if err != nil {
		return fmt.Errorf("encode ratchet: %w", err)
	}
	return fsys.WriteFile(filepath.Join(root, ratchetPath), append(data, '\n'))
}
