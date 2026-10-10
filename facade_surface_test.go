package evo_test

import (
	"errors"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// surfaceFacades are the facades whose exported surface must not leak a
// system type.
var surfaceFacades = []string{"internal/fs", "internal/process", "internal/terminal", "internal/clock"}

// surfaceProbeFile is the file name probe sources are type-checked under; its
// directory is the test's working directory, so module imports resolve.
const surfaceProbeFile = "zzprobe.go"

// surfaceTypes type-checks facade packages and probe sources from source with
// one shared importer, so the standard library is checked once.
type surfaceTypes struct {
	fset     *token.FileSet
	importer types.ImporterFrom
	dir      string
	banned   []resolvedSystemType
}

// loadSurfaceTypes is the once-per-process setup of surfaceTypes: the source
// importer and the resolved banned types.
var loadSurfaceTypes = sync.OnceValues(func() (*surfaceTypes, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("working directory: %w", err)
	}
	fset := token.NewFileSet()
	imp, ok := importer.ForCompiler(fset, "source", nil).(types.ImporterFrom)
	if !ok {
		return nil, errors.New("source importer does not resolve module imports")
	}
	loaded := &surfaceTypes{fset: fset, importer: imp, dir: dir}
	for _, system := range bannedSystemTypes {
		pkg, importErr := imp.ImportFrom(system.importPath, dir, 0)
		if importErr != nil {
			return nil, fmt.Errorf("import %s: %w", system.importPath, importErr)
		}
		named, isNamed := pkg.Scope().Lookup(system.name).Type().(*types.Named)
		if !isNamed {
			return nil, fmt.Errorf("%s is not a defined type", system.label())
		}
		loaded.banned = append(loaded.banned, resolvedSystemType{label: system.label(), named: named})
	}
	return loaded, nil
})

func (s *surfaceTypes) importPackage(importPath string) (*types.Package, error) {
	return s.importer.ImportFrom(importPath, s.dir, 0)
}

// checkProbe type-checks src as a package of this module.
func (s *surfaceTypes) checkProbe(src string) (*types.Package, error) {
	file, err := parser.ParseFile(s.fset, filepath.Join(s.dir, surfaceProbeFile), src, 0)
	if err != nil {
		return nil, fmt.Errorf("parse probe: %w", err)
	}
	config := types.Config{Importer: s.importer}
	return config.Check(modulePath+"/internal/zzprobe", s.fset, []*ast.File{file}, nil)
}

func (s *surfaceTypes) leaksOf(pkg *types.Package) []string {
	return newSurfaceWalker(s.fset, s.banned).leaksOf(pkg)
}

// TestFacadeSurfaceLeaksNoSystemTypes fails when the exported surface of
// internal/fs, internal/process, internal/terminal or internal/clock reaches
// os.File, os.Process, os.Signal or exec.Cmd through any signature, variable,
// constant, alias, defined type, struct field, embedding or interface method.
// The one exemption is the process.Signal alias.
func TestFacadeSurfaceLeaksNoSystemTypes(t *testing.T) {
	loaded, err := loadSurfaceTypes()
	if err != nil {
		t.Fatalf("load types: %v", err)
	}
	var leaks []string
	for _, facade := range surfaceFacades {
		pkg, importErr := loaded.importPackage(modulePath + "/" + facade)
		if importErr != nil {
			t.Fatalf("type-check %s: %v", facade, importErr)
		}
		leaks = append(leaks, loaded.leaksOf(pkg)...)
	}
	if len(leaks) > 0 {
		t.Errorf("%d exported facade declarations leak a system type:\n  %s",
			len(leaks), strings.Join(leaks, "\n  "))
	}
}

// surfaceProbes are probe packages with the number of leaks the guard must
// report. They are the guard's own proof: each evasion shape that slipped past
// the syntax-based guard, plus the allowed shapes it must keep allowing.
var surfaceProbes = []struct {
	name  string
	src   string
	leaks int
}{
	{"direct file result", "package p\nimport \"os\"\nfunc Open() *os.File { return nil }", 1},
	{"direct signal param", "package p\nimport \"os\"\nfunc Send(s os.Signal) {}", 1},
	{"direct command result", "package p\nimport \"os/exec\"\nfunc Cmd() *exec.Cmd { return nil }", 1},
	{"direct process result", "package p\nimport \"os\"\nfunc Start() *os.Process { return nil }", 1},
	{"exported alias", "package p\nimport \"os\"\ntype Alias = os.File", 1},
	{"unexported alias chain", "package p\nimport \"os\"\ntype handle = *os.File\nfunc Open() handle { return nil }", 1},
	{"exported struct field", "package p\nimport \"os\"\ntype Holder struct{ F *os.File }", 1},
	{"embedding through unexported alias", "package p\nimport \"os\"\ntype handle = *os.File\ntype Embed struct{ handle }", 1},
	{"defined type", "package p\nimport \"os\"\ntype Defined os.File", 1},
	{"interface method", "package p\nimport \"os\"\ntype Iface interface{ Handle() *os.File }", 1},
	{"exported var", "package p\nimport \"os\"\nvar Out = os.Stdout", 1},
	{"unexported type reached from exported func", "package p\nimport \"os\"\ntype hidden struct{ F *os.File }\nfunc Get() hidden { return hidden{} }", 1},
	{"method on exported type", "package p\nimport \"os\"\ntype T struct{}\nfunc (T) Raw() *os.File { return nil }", 1},
	{"file info through os spelling is allowed", "package p\nimport \"os\"\nfunc Info() os.FileInfo { return nil }", 0},
	{"file info through io/fs spelling is allowed", "package p\nimport iofs \"io/fs\"\nfunc Info() iofs.FileInfo { return nil }", 0},
	{"unexported handle field is allowed", "package p\nimport \"os\"\ntype Wrapper struct{ f *os.File }\nfunc (w Wrapper) Name() string { return w.f.Name() }", 0},
	{"process.Signal alias is allowed", "package p\nimport \"" + modulePath + "/internal/process\"\nfunc Wait(s process.Signal) {}\nvar Stop process.Signal", 0},
}

// TestFacadeSurfaceGuardCatchesEvasions proves the guard against probe
// packages, so a weakened walk fails here before it lets a real leak through.
func TestFacadeSurfaceGuardCatchesEvasions(t *testing.T) {
	loaded, err := loadSurfaceTypes()
	if err != nil {
		t.Fatalf("load types: %v", err)
	}
	for _, probe := range surfaceProbes {
		t.Run(probe.name, func(t *testing.T) {
			pkg, checkErr := loaded.checkProbe(probe.src)
			if checkErr != nil {
				t.Fatalf("type-check probe: %v", checkErr)
			}
			if got := loaded.leaksOf(pkg); len(got) != probe.leaks {
				t.Errorf("leaks = %d, want %d:\n  %s", len(got), probe.leaks, strings.Join(got, "\n  "))
			}
		})
	}
}
