//go:build unix

package publish

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/publish/cacheroot/cacherootest"
)

func lockOnce(t *testing.T, dest string) *Hold {
	t.Helper()
	hold, err := Lock(context.Background(), dest)
	if err != nil {
		t.Fatalf("lock %s: %v", dest, err)
	}
	return hold
}

func TestLockDirRecreatedAfterDeletion(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(base, "f")
	if err := lockOnce(t, dest).Release(); err != nil {
		t.Fatal(err)
	}
	dir, err := lockDir()
	if err != nil {
		t.Fatal(err)
	}
	cacherootest.RequireUnderScratch(t, dir)
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	hold := lockOnce(t, dest)
	defer func() { _ = hold.Release() }()
	other, ok, err := tryPathLock(dest)
	if err != nil || ok {
		_ = other.release()
		t.Fatalf("second claim while held: ok=%v err=%v; want serialized", ok, err)
	}
}

func TestLockDirRecreatedConcurrently(t *testing.T) {
	root := t.TempDir()
	dir, err := lockDir()
	if err != nil {
		t.Fatal(err)
	}
	cacherootest.RequireUnderScratch(t, dir)
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, name := range []string{"a", "b"} {
		wg.Go(func() {
			hold, err := Lock(context.Background(), filepath.Join(root, name))
			if err == nil {
				err = hold.Release()
			}
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestLockDirPruneStageRootRecreatedAfterDeletion(t *testing.T) {
	root := stagingRoot()
	if root == "" {
		t.Skip("no staging root")
	}
	cacherootest.RequireUnderScratch(t, root)
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	s := &Staged{dest: filepath.Join(t.TempDir(), "d"), apart: true}
	temp, err := createStagingDir(s.stagingPath)
	if err != nil {
		t.Fatalf("stage after root deleted: %v", err)
	}
	_ = os.RemoveAll(temp)
}
