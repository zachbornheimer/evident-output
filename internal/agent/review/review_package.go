package review

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"maps"
	"slices"
	"strings"
)

// GoPackage reviews multiple Go files in one package against the current
// rec dialect (see GoPackageAt).
func GoPackage(files map[string]string) Result {
	return GoPackageAt(files, "")
}

// GoPackageAt reviews multiple Go files in one package as they would be
// written for desiredVersion (empty means the current rec dialect).
// files maps filename → source. Files are reviewed in name order, so the
// findings keep one order across calls. The package's own declarations are
// type-checked across files without loading any import (MCP-017).
func GoPackageAt(files map[string]string, desiredVersion string) Result {
	if len(files) == 0 {
		return newResult([]Finding{{RuleID: "API-000", Message: "no files provided"}})
	}
	pkg := parsePackage(files)
	pkgHasEvo := pkg.importsEvo()
	var all []Finding
	for _, name := range pkg.names {
		f := pkg.parsed[name]
		if f == nil {
			continue
		}
		all = append(all, reviewFile(name, files[name], f, pkg.fset, desiredVersion)...)
		if pkgHasEvo {
			all = append(all, crossFileStreamFindings(name, f)...)
		}
	}
	all = append(all, pkg.parseErrors...)
	localErr := pkg.localTypeError()
	if localErr != "" {
		all = append(all, Finding{
			RuleID:  "MCP-017",
			Message: "cross-file typecheck incomplete: " + localErr,
		})
	}
	res := newResult(all)
	res.Partial = len(pkg.files) == 0 || localErr != ""
	res.DesiredVersion = desiredVersion
	return res
}

// parsedPackage is one package's files, each parsed once and shared by
// the per-file detectors and the type check.
type parsedPackage struct {
	fset        *token.FileSet
	names       []string
	parsed      map[string]*ast.File
	files       []*ast.File
	parseErrors []Finding
	name        string
}

func parsePackage(files map[string]string) parsedPackage {
	pkg := parsedPackage{
		fset:   token.NewFileSet(),
		names:  slices.Sorted(maps.Keys(files)),
		parsed: map[string]*ast.File{},
		name:   "main",
	}
	for _, name := range pkg.names {
		f, err := parser.ParseFile(pkg.fset, name, files[name], parser.SkipObjectResolution)
		if err != nil {
			pkg.parseErrors = append(pkg.parseErrors, parseErrorFinding(name, err))
			continue
		}
		pkg.name = f.Name.Name
		pkg.parsed[name] = f
		pkg.files = append(pkg.files, f)
	}
	return pkg
}

// localTypeError type-checks the package's own declarations and returns
// the first error that is not caused by an unloaded import, or "". Imports
// are never loaded, so every selector through one is unresolved by design;
// those errors say nothing about the reviewed code.
func (pkg parsedPackage) localTypeError() string {
	qualifiers := pkg.importQualifiers()
	var first string
	conf := types.Config{
		Importer: emptyImporter{},
		Error: func(err error) {
			te, ok := err.(types.Error)
			if first != "" || (ok && qualifiers[te.Pos]) {
				return
			}
			if ok && strings.Contains(te.Msg, "imported and not used") {
				return
			}
			first = err.Error()
		},
	}
	_, _ = conf.Check(pkg.name, pkg.fset, pkg.files, nil)
	return first
}

// importQualifiers is the position of every package qualifier in the
// package: the X of a selector whose name is one of its file's imports.
func (pkg parsedPackage) importQualifiers() map[token.Pos]bool {
	out := map[token.Pos]bool{}
	for _, f := range pkg.files {
		names := importNames(f)
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if id, ok := sel.X.(*ast.Ident); ok && names[id.Name] {
				out[id.Pos()] = true
				out[sel.Sel.Pos()] = true
			}
			return true
		})
	}
	return out
}

// importNames is the set of names file f can qualify an import by: its
// alias, else the last path element, and evo for this module.
func importNames(f *ast.File) map[string]bool {
	names := map[string]bool{}
	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if imp.Name != nil {
			names[imp.Name.Name] = true
			continue
		}
		names[path[strings.LastIndex(path, "/")+1:]] = true
	}
	if name := evoImportName(f); name != "" {
		names[name] = true
	}
	return names
}

// importsEvo reports whether any parsed file imports evo, so STREAM rules
// apply across the package.
func (pkg parsedPackage) importsEvo() bool {
	for _, f := range pkg.files {
		if evoImportName(f) != "" {
			return true
		}
	}
	return false
}

// crossFileStreamFindings flags fmt.Print* in a file that does not import
// evo itself, in a package that does (STREAM-003).
func crossFileStreamFindings(name string, f *ast.File) []Finding {
	if evoImportName(f) != "" || !callsFmtPrint(f) {
		return nil
	}
	return []Finding{{
		RuleID:  "STREAM-003",
		Message: "fmt.Print* in package that imports evo may contaminate managed streams (cross-file)",
		File:    name,
	}}
}

// callsFmtPrint reports whether f calls fmt.Print* or fmt.Fprint*.
func callsFmtPrint(f *ast.File) bool {
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if ok && identName(sel.X) == "fmt" &&
			(strings.HasPrefix(sel.Sel.Name, "Print") || strings.HasPrefix(sel.Sel.Name, "Fprint")) {
			found = true
		}
		return !found
	})
	return found
}

// emptyImporter gives go/types an empty package for every import, so the
// check stays local to the reviewed sources.
type emptyImporter struct{}

func (emptyImporter) Import(path string) (*types.Package, error) {
	return types.NewPackage(path, path[strings.LastIndex(path, "/")+1:]), nil
}
