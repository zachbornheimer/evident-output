package freshness

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestObservedFileMatchesFSPath proves ObservedFile names exactly the
// identity FSPath would observe for a regular file holding those bytes, so
// a caller that already read the bytes records a Basis a later FSPath
// revalidation agrees with.
func TestObservedFileMatchesFSPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.txt")
	contents := []byte("hello\n")
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatal(err)
	}
	want, err := FSPath(path).Fingerprint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := ObservedFile(path, contents); got != want {
		t.Fatalf("ObservedFile = %+v, FSPath = %+v", got, want)
	}
	if got := ObservedFile(path, []byte("other\n")); got.Digest == want.Digest {
		t.Fatal("different bytes produced the same digest")
	}
}

// TestObservedMissingMatchesFSPath proves ObservedMissing names exactly the
// identity FSPath observes for an absent path.
func TestObservedMissingMatchesFSPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.txt")
	want, err := FSPath(path).Fingerprint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := ObservedMissing(path); got != want {
		t.Fatalf("ObservedMissing = %+v, FSPath = %+v", got, want)
	}
	if ObservedFile(path, nil).Digest == want.Digest {
		t.Fatal("an empty file collided with a missing path")
	}
}
