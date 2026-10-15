package apisurface

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeclFiles_KeysExportedDeclarationsByFile(t *testing.T) {
	dir := t.TempDir()
	write := func(name, src string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.go", `package p

type Alpha struct{}
type hidden struct{}

func (a *Alpha) Run() {}
func (a Alpha) stop()  {}
func Start()           {}

const (
	One = 1
	two = 2
)
`)
	write("b.go", `package p

type Box[T any] struct{}

func (b *Box[T]) Put(v T) {}

var Global, other = 1, 2
`)
	write("a_test.go", `package p

func TestOnly() {}
func Helper()   {}
`)

	got, err := DeclFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"Alpha": "a.go", "Alpha.Run": "a.go", "Start": "a.go", "One": "a.go",
		"Box": "b.go", "Box.Put": "b.go", "Global": "b.go",
	}
	if len(got) != len(want) {
		t.Errorf("got %v, want %v", got, want)
	}
	for name, file := range want {
		if got[name] != file {
			t.Errorf("%s declared in %q, want %q", name, got[name], file)
		}
	}
}

func TestDeclFiles_ParseErrorNamesTheFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.go"), []byte("package"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := DeclFiles(dir); err == nil {
		t.Fatal("want a parse error")
	}
}
