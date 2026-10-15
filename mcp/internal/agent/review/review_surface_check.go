package review

import (
	_ "embed"
	"errors"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"regexp"
	"strings"
	"sync"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/modpin"
)

// surfaceStubSource is the declaration-only 2.0 surface (apisurface.Stub of
// the root package). It lets review type-check a program against the real
// 2.0 API from inside the MCP binary, whatever module the program sits in.
// TestSurfaceStubIsCurrent keeps it equal to the package.
//
//go:embed surface_2_0.txt
var surfaceStubSource string

// surfaceChecker type-checks programs against the stub. The source importer
// caches every standard-library package it loads, so one instance serves
// all reviews; it is not safe for concurrent use, hence the mutex.
var surfaceChecker struct {
	sync.Mutex
	std  types.Importer
	stub *types.Package
}

// compileError is one type error of a program that imports the 2.0 module.
type compileError struct {
	pos token.Pos
	msg string
}

// fragmentNoise are type errors that say the reviewed file is a fragment of
// a larger package or has local slips, not that it misuses the 2.0 API: an
// application symbol the file never declares, an unused name, an import the
// importer could not load.
var fragmentNoise = regexp.MustCompile(
	`^undefined: [A-Za-z_]\w*$|declared and not used|is not used|imported and not used|missing return|could not import`)

// surfaceImporter resolves the 2.0 module to the stub and the standard
// library to its source; any other package is an empty stand-in, so errors
// that depend on one are not trusted (see surfaceCompileErrors).
type surfaceImporter struct {
	std    types.Importer
	stub   *types.Package
	faked  bool
	byPath map[string]*types.Package
}

func (s *surfaceImporter) Import(path string) (*types.Package, error) {
	if path == modpin.ModulePath && s.stub != nil {
		return s.stub, nil
	}
	if !strings.Contains(strings.SplitN(path, "/", 2)[0], ".") {
		if pkg, err := s.std.Import(path); err == nil {
			return pkg, nil
		}
	}
	s.faked = true
	if pkg, ok := s.byPath[path]; ok {
		return pkg, nil
	}
	pkg := types.NewPackage(path, path[strings.LastIndex(path, "/")+1:])
	pkg.MarkComplete()
	s.byPath[path] = pkg
	return pkg, nil
}

// loadSurface type-checks the embedded stub once.
func loadSurface() (types.Importer, *types.Package) {
	if surfaceChecker.std == nil {
		surfaceChecker.std = importer.ForCompiler(token.NewFileSet(), "source", nil)
		fset := token.NewFileSet()
		if file, err := parser.ParseFile(fset, "surface_2_0.go", surfaceStubSource, 0); err == nil {
			imp := &surfaceImporter{std: surfaceChecker.std, byPath: map[string]*types.Package{}}
			conf := types.Config{Importer: imp, Error: func(error) {}}
			surfaceChecker.stub, _ = conf.Check(modpin.ModulePath, fset, []*ast.File{file}, nil)
		}
	}
	return surfaceChecker.std, surfaceChecker.stub
}

// surfaceCompileErrors type-checks file against the 2.0 surface and returns
// the errors that mean the program cannot compile against it. alias is the
// local name of the 2.0 import. When a package other than the standard
// library is imported, only errors that name the alias are kept, since the
// rest may stem from the stand-in.
func surfaceCompileErrors(fset *token.FileSet, file *ast.File, alias string) []compileError {
	surfaceChecker.Lock()
	defer surfaceChecker.Unlock()
	std, stub := loadSurface()
	if stub == nil {
		return nil
	}
	imp := &surfaceImporter{std: std, stub: stub, byPath: map[string]*types.Package{}}
	var all []compileError
	conf := types.Config{Importer: imp, FakeImportC: true, DisableUnusedImportCheck: true, Error: func(err error) {
		var typeErr types.Error
		if errors.As(err, &typeErr) && !fragmentNoise.MatchString(typeErr.Msg) {
			all = append(all, compileError{pos: typeErr.Pos, msg: typeErr.Msg})
		}
	}}
	_, _ = conf.Check("reviewed", fset, []*ast.File{file}, nil)
	if !imp.faked {
		return all
	}
	var named []compileError
	for _, e := range all {
		if strings.Contains(e.msg, alias+".") || strings.Contains(e.msg, "evo.") {
			named = append(named, e)
		}
	}
	return named
}

// reportCompileErrors reports each compile error against the 2.0 surface
// that no rewrite above already covers, so a program that does not compile
// is never clean. The replacement comes from the retired table when the
// error names a removed identifier.
func (d *removedTwoZeroDetector) reportCompileErrors() {
	if !d.importsTwoZero() {
		return
	}
	for _, e := range surfaceCompileErrors(d.fset, d.file, d.pkg) {
		off := d.fset.File(e.pos).Offset(e.pos)
		if slicesContainsSpan(d.reported, off) {
			continue
		}
		message, suggestion := "does not compile against the 2.0 surface: "+e.msg, "make the call match the 2.0 signature in CONTRACT.md"
		if removed, ok := removedNamedIn(e.msg); ok {
			message = removed.Contract + " was removed in " + string(removed.RemovedIn) + " and does not compile against the 2.0 surface: " + e.msg
			suggestion = "instead: " + removed.Replacement
		}
		d.findings = append(d.findings, Finding{
			RuleID: "API-032", Message: message, File: d.filename,
			Line: lineAt(d.src, off), Suggestion: suggestion,
		})
	}
}

func (d *removedTwoZeroDetector) importsTwoZero() bool {
	for _, imp := range d.file.Imports {
		if strings.Trim(imp.Path.Value, `"`) == modpin.ModulePath {
			return true
		}
	}
	return false
}

func slicesContainsSpan(spans []srcSpan, off int) bool {
	for _, s := range spans {
		if s.contains(off) {
			return true
		}
	}
	return false
}
