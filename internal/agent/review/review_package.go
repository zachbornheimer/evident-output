package review

import (
	"fmt"
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
// written for desiredVersion (empty means the current rec dialect), with
// go/types for cross-file API resolution without executing package code
// (MCP-017). files maps filename → source. External imports are stubbed
// so type-check stays local to the provided sources. Files are reviewed in
// name order, so the findings keep one order across calls.
func GoPackageAt(files map[string]string, desiredVersion string) Result {
	if len(files) == 0 {
		return newResult([]Finding{{RuleID: "API-000", Message: "no files provided"}})
	}
	pkg := parsePackage(files)
	pkgHasEvo := packageImportsEvo(files)
	var all []Finding
	for _, name := range pkg.names {
		src := files[name]
		if f := pkg.parsed[name]; f != nil {
			all = append(all, reviewFile(name, src, f, pkg.fset, desiredVersion)...)
		}
		if pkgHasEvo {
			all = append(all, crossFileStreamFindings(name, src)...)
		}
	}
	all = append(all, pkg.parseErrors...)
	if len(pkg.files) == 0 {
		res := newResult(all)
		res.Partial = true
		res.DesiredVersion = desiredVersion
		return res
	}
	typed, typeErr := pkg.typeCheck()
	all = append(all, admitDialect(typedCollectionLeafFindings(pkg, typed), desiredVersion)...)
	// Partial when the type check failed: stubbed imports can leave
	// cross-file types unresolved.
	partial := typeErr != nil && (pkgHasEvo || len(files) >= 2)
	if typeErr != nil && len(files) >= 2 {
		all = append(all, Finding{
			RuleID:  "MCP-017",
			Message: "cross-file typecheck incomplete: " + typeErr.Error(),
		})
	}
	res := newResult(all)
	res.Partial = partial
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

// typeCheck resolves the package's types locally. The returned info is
// usable even when the check reports an error.
func (pkg parsedPackage) typeCheck() (*types.Info, error) {
	conf := types.Config{
		// Local-only: missing imports do not abort the whole check.
		Importer: stubImporter{},
		Error:    func(error) {}, // collect via Check return
	}
	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
		Uses:  make(map[*ast.Ident]types.Object),
		Defs:  make(map[*ast.Ident]types.Object),
	}
	_, err := conf.Check(pkg.name, pkg.fset, pkg.files, info)
	return info, err
}

// packageImportsEvo reports whether any file imports evo, so STREAM rules
// apply across the package.
func packageImportsEvo(files map[string]string) bool {
	for _, src := range files {
		if strings.Contains(src, "evident-output") || strings.Contains(src, `"evo"`) {
			return true
		}
	}
	return false
}

// crossFileStreamFindings flags fmt.Print* in a file that does not import
// evo itself, in a package that does (STREAM-003).
func crossFileStreamFindings(name, src string) []Finding {
	if strings.Contains(src, "evident-output") {
		return nil
	}
	if !strings.Contains(src, "fmt.Print") && !strings.Contains(src, "fmt.Fprint") {
		return nil
	}
	return []Finding{{
		RuleID:  "STREAM-003",
		Message: "fmt.Print* in package that imports evo may contaminate managed streams (cross-file)",
		File:    name,
	}}
}

// collectionLeafVerbs are the leaf verbs a Group/Sequence must not call.
var collectionLeafVerbs = []string{"Done", "Fail", "Progress"}

// typedCollectionLeafFindings flags a leaf verb called on a value whose
// resolved type is a Group or Sequence handle (API-027).
func typedCollectionLeafFindings(pkg parsedPackage, info *types.Info) []Finding {
	var out []Finding
	for _, f := range pkg.files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !slices.Contains(collectionLeafVerbs, sel.Sel.Name) {
				return true
			}
			tv, ok := info.Types[sel.X]
			if !ok || tv.Type == nil {
				return true
			}
			tn := tv.Type.String()
			if strings.Contains(tn, "GroupHandle") || strings.Contains(tn, "SequenceHandle") {
				pos := pkg.fset.Position(n.Pos())
				out = append(out, Finding{
					RuleID:  "API-027",
					Message: fmt.Sprintf("typed: %s.%s on collection type %s is forbidden", tn, sel.Sel.Name, tn),
					File:    pos.Filename,
					Line:    pos.Line,
					Column:  pos.Column,
				})
			}
			return true
		})
	}
	return out
}

// stubImporter satisfies go/types for external imports without loading code.
type stubImporter struct{}

func (stubImporter) Import(path string) (*types.Package, error) {
	// Return an empty package so Check can continue for local symbols.
	return types.NewPackage(path, path[strings.LastIndex(path, "/")+1:]), nil
}
