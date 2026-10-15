package review

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/apisurface"
)

// rootDir is the module root, whose package is the 2.0 surface.
var rootDir = filepath.Join("..", "..", "..", "..")

const updateSurfaceStub = "UPDATE_SURFACE_STUB"

// TestSurfaceStubIsCurrent keeps surface_2_0.txt equal to the declarations
// of the root package, and proves the stub exports exactly the API
// contract's surface. Regenerate with UPDATE_SURFACE_STUB=1.
func TestSurfaceStubIsCurrent(t *testing.T) {
	want, err := apisurface.Stub(rootDir)
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv(updateSurfaceStub) != "" {
		if err := os.WriteFile("surface_2_0.txt", want, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if string(want) != surfaceStubSource {
		t.Fatalf("surface_2_0.txt is stale; regenerate it with %s=1 go test ./internal/agent/review -run TestSurfaceStubIsCurrent", updateSurfaceStub)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "surface.go"), want, 0o644); err != nil {
		t.Fatal(err)
	}
	live, err := apisurface.Walk(rootDir)
	if err != nil {
		t.Fatal(err)
	}
	stub, err := apisurface.Walk(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(live, stub) {
		t.Errorf("stub exports differ from the package's exported surface\nlive: %d lines, stub: %d lines", len(live), len(stub))
	}
	if _, pkg := loadSurface(); pkg == nil || !pkg.Complete() {
		t.Error("the stub does not type-check against the standard library")
	}
}
