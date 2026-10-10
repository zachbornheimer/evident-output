package freshness

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// recordingClaims is a Claimer that notes each path it was asked to hold, and
// whether the claim was still held while the observation ran.
type recordingClaims struct {
	mu     sync.Mutex
	paths  []string
	active int
	nested bool
}

func (c *recordingClaims) HoldRead(ctx context.Context, path string, observe func(context.Context) error) error {
	c.mu.Lock()
	c.paths = append(c.paths, path)
	c.active++
	c.nested = c.nested || c.active > 1
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.active--
		c.mu.Unlock()
	}()
	return observe(ctx)
}

// TestBasisRecordsRejectDuplicateKind proves spec §11.1: duplicate (Kind,
// Key) Basis pairs within one operation are a programmer error.
func TestBasisRecordsRejectDuplicateKind(t *testing.T) {
	_, err := basisRecordsFrom(context.Background(), []Fingerprint{Value("same", 1), Value("same", 2)})
	if err == nil {
		t.Fatal("duplicate (kind,key) Basis entries must be rejected")
	}
}

// TestObserveBasisClaimsEachFilesystemPathOnceAndNeverNested proves ZYS-840:
// filesystem entries are observed under a read claim taken one at a time,
// and a Value, which touches no path, takes none.
func TestObserveBasisClaimsEachFilesystemPathOnceAndNeverNested(t *testing.T) {
	dir := t.TempDir()
	first, second := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	for _, p := range []string{first, second} {
		if err := os.WriteFile(p, []byte(p), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	claims := &recordingClaims{}
	records, err := ObserveBasis(context.Background(), claims, []Fingerprint{FSPath(second), Value("v", 1), FSPath(first)})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatalf("records = %d, want 3", len(records))
	}
	if len(claims.paths) != 2 || claims.nested {
		t.Fatalf("claims = %v nested=%v, want exactly the two paths, one at a time", claims.paths, claims.nested)
	}
}

// TestObserveBasisIsCanonicalAcrossInputOrder proves spec §11.1: Basis order
// is not identity.
func TestObserveBasisIsCanonicalAcrossInputOrder(t *testing.T) {
	forward, err := ObserveBasis(context.Background(), &recordingClaims{}, []Fingerprint{Value("a", 1), Value("b", 2)})
	if err != nil {
		t.Fatal(err)
	}
	backward, err := ObserveBasis(context.Background(), &recordingClaims{}, []Fingerprint{Value("b", 2), Value("a", 1)})
	if err != nil {
		t.Fatal(err)
	}
	if !BasisRecordsEqual(forward, backward) {
		t.Fatalf("Basis order changed identity: %+v vs %+v", forward, backward)
	}
}
