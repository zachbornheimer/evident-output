package review_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

func TestGoDirectory_MergesTwoFiles(t *testing.T) {
	dir := t.TempDir()
	a := `package a
import evo "github.com/zachbornheimer/evident-output"
func f(out *evo.Output) { out.Plan("a") }
`
	b := `package b
import evo "github.com/zachbornheimer/evident-output"
func f(out *evo.Output) { out.Changes("b") }
`
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte(a), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.go"), []byte(b), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := review.GoDirectory(dir)
	if err != nil {
		t.Fatalf("GoDirectory: %v", err)
	}
	var sawA, sawB bool
	for _, f := range res.Findings {
		if f.RuleID != "API-032" {
			continue
		}
		if strings.Contains(f.File, "a.go") {
			sawA = true
		}
		if strings.Contains(f.File, "b.go") {
			sawB = true
		}
	}
	if !sawA || !sawB {
		t.Fatalf("directory review must merge both files, got %+v", res.Findings)
	}
}

func TestGoDirectory_FillsPinFromGoMod(t *testing.T) {
	dir := t.TempDir()
	mod := "module app\n\nrequire github.com/zachbornheimer/evident-output v0.4.6\nreplace github.com/zachbornheimer/evident-output => ./evo\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "evo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := review.GoDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.ModuleVersion != "v0.4.6" || res.DesiredVersion != "v0.4.6" {
		t.Fatalf("pin fields: %+v", res)
	}
	if res.ReplacePath != filepath.Join(dir, "evo") {
		t.Fatalf("ReplacePath=%q", res.ReplacePath)
	}
}
