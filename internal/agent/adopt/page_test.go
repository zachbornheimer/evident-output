package adopt_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/adopt"
)

// TestInventoryPage_MixedIsPagedPlan proves testdata/mixed still finds
// spinner/fmt/log.Fatal, but as a paged plan: one rung, cap 40, next_action set.
func TestInventoryPage_MixedIsPagedPlan(t *testing.T) {
	page, err := adopt.InventoryPage(filepath.Join("testdata", "mixed"), adopt.InventoryOptions{})
	if err != nil {
		t.Fatalf("InventoryPage: %v", err)
	}
	if len(page.Findings) == 0 || len(page.Findings) > adopt.DefaultPageSize {
		t.Fatalf("findings = %d, want 1..%d", len(page.Findings), adopt.DefaultPageSize)
	}
	if page.Rung == "" {
		t.Fatal("paged mixed inventory must name the current rung")
	}
	if page.NextAction == "" {
		t.Fatal("paged mixed inventory must set next_action")
	}
	if page.NextAction == adopt.NextActionClean {
		t.Fatal("mixed fixture is not a clean inventory")
	}
}

// TestInventoryPage_CursorIsNotPageOne proves re-calling with next_cursor
// advances: page 2 is never page 1 again.
func TestInventoryPage_CursorIsNotPageOne(t *testing.T) {
	first, err := adopt.InventoryPage(filepath.Join("testdata", "mixed"), adopt.InventoryOptions{})
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if first.NextCursor == "" {
		t.Fatal("mixed has more than one rung; want a next_cursor")
	}
	second, err := adopt.InventoryPage(filepath.Join("testdata", "mixed"), adopt.InventoryOptions{
		Cursor: first.NextCursor,
	})
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if second.Rung == first.Rung && sameFindingLines(first.Findings, second.Findings) {
		t.Fatalf("page 2 repeated page 1: rung=%s findings=%+v", second.Rung, second.Findings)
	}
	if len(second.Findings) == 0 {
		t.Fatal("page 2 of mixed must still have findings")
	}
}

// TestInventoryPage_EmptyIsClean proves an empty tree is a done plan, not a dump.
func TestInventoryPage_EmptyIsClean(t *testing.T) {
	dir := t.TempDir()
	src := "package p\nfunc F() {}\n"
	if err := os.WriteFile(filepath.Join(dir, "empty.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	page, err := adopt.InventoryPage(dir, adopt.InventoryOptions{})
	if err != nil {
		t.Fatalf("InventoryPage: %v", err)
	}
	if page.NextAction != adopt.NextActionClean {
		t.Errorf("next_action = %q, want %q", page.NextAction, adopt.NextActionClean)
	}
	if len(page.Findings) != 0 {
		t.Errorf("clean inventory has findings: %+v", page.Findings)
	}
	if page.Remaining != 0 {
		t.Errorf("remaining = %d, want 0", page.Remaining)
	}
	if page.NextCursor != "" {
		t.Errorf("clean inventory must not offer a next_cursor, got %q", page.NextCursor)
	}
}

// TestInventoryPage_FacadeTreePagesFacadesOnly proves a tree with a facade
// yields the facade page, not the 2324-style call sites.
func TestInventoryPage_FacadeTreePagesFacadesOnly(t *testing.T) {
	page, err := adopt.InventoryPage(filepath.Join("testdata", "facade"), adopt.InventoryOptions{})
	if err != nil {
		t.Fatalf("InventoryPage: %v", err)
	}
	if len(page.Facades) != 1 {
		t.Fatalf("want 1 facade, got %d: %+v", len(page.Facades), page.Facades)
	}
	if page.NextAction != adopt.NextActionFacade {
		t.Errorf("next_action = %q, want %q", page.NextAction, adopt.NextActionFacade)
	}
	if len(page.Findings) != 0 {
		t.Errorf("facade page must not dump call-site findings, got %+v", page.Findings)
	}
}

func sameFindingLines(a, b []adopt.Finding) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].File != b[i].File || a[i].Line != b[i].Line || a[i].Pattern != b[i].Pattern {
			return false
		}
	}
	return true
}
