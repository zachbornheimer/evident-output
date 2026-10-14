// Command genschema writes schema/run.v3.json and schema/event.v2.json from
// the Go wire types in internal/project/wire. Run from the module root:
//
//	go run ./tools/genschema
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/zachbornheimer/evident-output/v2/internal/project/wire"
)

func main() {
	root, err := moduleRoot()
	if err != nil {
		fail(err)
	}
	files := map[string][]byte{
		filepath.Join("schema", "run.v3.json"):   wire.RunJSONSchema(),
		filepath.Join("schema", "event.v2.json"): wire.EventJSONSchema(),
	}
	for rel, body := range files {
		dest := filepath.Join(root, rel)
		if err := os.WriteFile(dest, body, 0o600); err != nil {
			fail(fmt.Errorf("write %s: %w", rel, err))
		}
		fmt.Fprintf(os.Stderr, "genschema: wrote %s\n", rel)
	}
}

func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above %s", dir)
		}
		dir = parent
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "genschema:", err)
	os.Exit(1)
}
