package main

import (
	"fmt"
	"os"
)

const (
	goTestTimeout = "5m"
	goTestPackage = "2"
	filePerm      = 0o644
)

type scratchSandbox struct{}

// ApplyPatch applies the patch with `git apply`, which refuses a patch whose
// context no longer matches the tree.
func (scratchSandbox) ApplyPatch(dir string, patch []byte) error {
	return gitApply(dir, patch)
}

func (scratchSandbox) RunTests(dir, pkg string, names []string) ([]byte, error) {
	return goTestJSON(dir, pkg, names)
}

func tempScratch() (string, func(), error) {
	dir, err := os.MkdirTemp("", "red-proof-")
	if err != nil {
		return "", nil, fmt.Errorf("create temp directory: %w", err)
	}
	return dir, func() { _ = os.RemoveAll(dir) }, nil
}
