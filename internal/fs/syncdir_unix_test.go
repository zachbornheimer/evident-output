//go:build !windows

package fs

import (
	"path/filepath"
	"testing"
)

func TestSyncDirSyncsExistingDirectoryAndReportsMissingOne(t *testing.T) {
	dir := t.TempDir()
	if err := SyncDir(dir); err != nil {
		t.Fatalf("SyncDir(existing) = %v, want nil", err)
	}
	if err := SyncDir(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("SyncDir(missing) = nil, want an error")
	}
}
