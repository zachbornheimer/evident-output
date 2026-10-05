package manifest

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
	path, err := Locate(Config{AppID: "app", Workspace: "/w"}, NewOSEnvironment())
	restore()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(path, dir+string(filepath.Separator)) {
		t.Fatalf("Locate = %q, want a path under %q", path, dir)
	}
	after, err := Locate(Config{AppID: "app", Workspace: "/w"}, NewOSEnvironment())
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(after, dir) {
		t.Fatalf("Locate after restore = %q, still under the redirect", after)
	}
}
