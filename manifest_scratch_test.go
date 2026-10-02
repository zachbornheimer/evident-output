package evo_test

import (
	"testing"

	"github.com/zachbornheimer/evident-output/internal/manifest"
	"github.com/zachbornheimer/evident-output/internal/publish/cacheroot/cacherootest"
)

// TestDefaultManifestLivesUnderScratchCacheRoot proves this package's
// tests never read, write, or wait on a manifest in the real user cache:
// the default manifest location resolves under the scratch cache root
// TestMain set up, which child processes inherit.
func TestDefaultManifestLivesUnderScratchCacheRoot(t *testing.T) {
	path, err := manifest.Locate(manifest.Config{Workspace: t.TempDir()}, manifest.NewOSEnvironment())
	if err != nil {
		t.Fatal(err)
	}
	cacherootest.RequireUnderScratch(t, path)
}
