package fingerprint

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFSPathRegularFileIsMtimeInsensitive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := FSPath(path).Fingerprint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, time.Now().Add(time.Hour), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	second, err := FSPath(path).Fingerprint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest != second.Digest {
		t.Fatalf("digest changed after mtime/mode-only edit: %x != %x", first.Digest, second.Digest)
	}
	if first.Kind != KindFSPath || first.Key != path {
		t.Fatalf("got Kind=%q Key=%q", first.Kind, first.Key)
	}
}

func TestFSPathRegularFileContentChangeChangesDigest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := FSPath(path).Fingerprint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := FSPath(path).Fingerprint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if before.Digest == after.Digest {
		t.Fatal("digest did not change after content change")
	}
}

func TestFSPathMissingIsStableAndDistinctFromPresent(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "nope")
	first, err := FSPath(missing).Fingerprint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := FSPath(missing).Fingerprint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest != second.Digest {
		t.Fatal("missing digest is not stable")
	}
	present := filepath.Join(dir, "present")
	if err := os.WriteFile(present, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	presentFP, err := FSPath(present).Fingerprint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest == presentFP.Digest {
		t.Fatal("missing digest collides with a present empty file")
	}
}

func TestFSPathInaccessibleIsErrorNotMissing(t *testing.T) {
	sentinel := os.ErrPermission
	withFS(fakeFS{lstatErr: sentinel}, func() {
		_, err := FSPath("/whatever").Fingerprint(context.Background())
		if err == nil {
			t.Fatal("expected an error for an inaccessible path")
		}
	})
}

func TestFSPathSymlinkFingerprintsTargetTextWithoutFollowing(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("real contents"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	linkFP, err := FSPath(link).Fingerprint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	targetFP, err := FSPath(target).Fingerprint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if linkFP.Digest == targetFP.Digest {
		t.Fatal("symlink fingerprint followed the target instead of hashing link text")
	}
	other := filepath.Join(dir, "link2")
	if err := os.Symlink(target, other); err != nil {
		t.Fatal(err)
	}
	other2FP, err := FSPath(other).Fingerprint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if linkFP.Digest != other2FP.Digest {
		t.Fatal("two symlinks with the same target text produced different digests")
	}
}

func TestFSPathDirectoryDeterministicAndMtimeInsensitive(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("A"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "b.txt"), []byte("B"), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := FSPath(dir).Fingerprint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(dir, "a.txt"), time.Now().Add(time.Hour), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(dir, "a.txt"), 0o600); err != nil {
		t.Fatal(err)
	}
	second, err := FSPath(dir).Fingerprint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest != second.Digest {
		t.Fatal("directory digest changed after mtime/mode-only edit to a member file")
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "b.txt"), []byte("B-changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	third, err := FSPath(dir).Fingerprint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second.Digest == third.Digest {
		t.Fatal("directory digest did not change after a nested file's contents changed")
	}
}

// fakeFS lets this package's own tests simulate an inaccessible path
// without depending on real filesystem permission behavior (which varies
// by platform/user, e.g. root ignoring 0000 permissions).
type fakeFS struct {
	lstatErr error
}

func (f fakeFS) Lstat(string) (fs.FileInfo, error)   { return nil, f.lstatErr }
func (fakeFS) ReadFile(string) ([]byte, error)       { return nil, os.ErrInvalid }
func (fakeFS) ReadDir(string) ([]fs.DirEntry, error) { return nil, os.ErrInvalid }
func (fakeFS) Readlink(string) (string, error)       { return "", os.ErrInvalid }
