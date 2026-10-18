package freshness

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestRedirectCacheDirMovesDefaultLocation proves RedirectCacheDir moves a
// default-located manifest under the redirect and restore undoes it.
func TestRedirectCacheDirMovesDefaultLocation(t *testing.T) {
	dir := t.TempDir()
	restore := RedirectCacheDir(dir)
	path, err := LocateManifest(ManifestConfig{AppID: "app", Workspace: "/w"}, NewSystemManifestEnvironment())
	restore()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(path, dir+string(filepath.Separator)) {
		t.Fatalf("LocateManifest = %q, want a path under %q", path, dir)
	}
	after, err := LocateManifest(ManifestConfig{AppID: "app", Workspace: "/w"}, NewSystemManifestEnvironment())
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(after, dir) {
		t.Fatalf("LocateManifest after restore = %q, still under the redirect", after)
	}
}
