package freshness_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/conformance/contract/harness"
)

func TestC10_010_ManifestPersistsTheFingerprintAlgorithm(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "in.txt")
	if err := os.WriteFile(input, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(dir, "state")
	out, _ := harness.New(t, func(c *evo.Config) { c.StateDir = state; c.AppID = "pending-manifest" })
	task := out.Task("write")
	task.Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{
			Path: filepath.Join(dir, "out.txt"), Contents: []byte("y"), Basis: []evo.Fingerprint{evo.FSPath(input)},
		})
	})
	_ = task.Wait()
	_ = out.Close()
	manifests, err := filepath.Glob(filepath.Join(state, "manifest-*.json"))
	if err != nil || len(manifests) != 1 {
		t.Fatalf("manifests = %v, %v", manifests, err)
	}
	raw, err := os.ReadFile(manifests[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(string(raw)), "algorithm") {
		t.Fatalf("manifest persists no fingerprint algorithm id:\n%s", raw)
	}
}
