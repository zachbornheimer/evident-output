package fs_test

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/fs"
)

// TestFileHidesTheOperatingSystemHandle proves a holder cannot type-assert a
// File back to the *os.File behind it, and that io.Copy still reaches the
// handle's ReadFrom fast path.
func TestFileHidesTheOperatingSystemHandle(t *testing.T) {
	f, err := fs.CreateExclusive(filepath.Join(t.TempDir(), "held"), 0o600)
	if err != nil {
		t.Fatalf("CreateExclusive: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	if _, leaked := f.(*os.File); leaked {
		t.Error("File is the *os.File itself: a holder can chmod, stat or signal past the facade")
	}
	if _, fast := f.(io.ReaderFrom); !fast {
		t.Error("File does not forward ReadFrom: io.Copy loses the kernel copy fast path")
	}
}

func TestFileAcceptsACopyAndKeepsItsName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "held")
	f, err := fs.CreateExclusive(path, 0o600)
	if err != nil {
		t.Fatalf("CreateExclusive: %v", err)
	}
	if _, err := io.Copy(f, strings.NewReader("payload")); err != nil {
		t.Fatalf("io.Copy: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if f.Name() != path {
		t.Errorf("Name = %q, want %q", f.Name(), path)
	}
	if got, err := fs.ReadFile(path); err != nil || string(got) != "payload" {
		t.Errorf("file holds %q, %v; want payload", got, err)
	}
}
