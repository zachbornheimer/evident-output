package manifest

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/publish/cacheroot"
)

// TestCacheRootMovesDefaultLocation proves a default-located manifest
// lives under the shared cache root, so pointing that root at scratch
// space keeps manifests out of the real user cache.
func TestCacheRootMovesDefaultLocation(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(cacheroot.EnvVar, dir)
	path, err := Locate(Config{AppID: "app", Workspace: "/w"}, NewOSEnvironment())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(path, dir+string(filepath.Separator)) {
		t.Fatalf("Locate = %q, want a path under %q", path, dir)
	}
}
