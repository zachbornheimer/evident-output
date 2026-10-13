package apisurface

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestWalk_ListsTypedConstantsWithTheirType(t *testing.T) {
	dir := t.TempDir()
	src := `package p

type Mode string

const (
	Fast Mode = "fast"
	Slow Mode = "slow"
	slow Mode = "hidden"
)

const Plain = 1
`
	if err := os.WriteFile(filepath.Join(dir, "p.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Walk(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"value Fast Mode", "value Slow Mode", "value Plain"} {
		if !slices.Contains(got, want) {
			t.Errorf("Walk lines %q missing %q", got, want)
		}
	}
	if slices.Contains(got, "value slow Mode") {
		t.Errorf("Walk listed unexported const: %q", got)
	}
}
