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
	// API-070/090/091/120 (removed-name findings) is the same
	// fix.RemovedNameAnalyzers path GoFileAt/GoDirectoryAt use — see
	// removedNamePackageFindings for why a files map with no shared disk
	// location can still resolve evo for real.
	removed, found, removedPartial := removedNamePackageFindings(files)
	if found {
		all = append(all, admitDialect(removed, desiredVersion)...)
	}
	res := newResult(all)
	res.Partial = len(pkg.files) == 0 || localErr != "" || (found && removedPartial) || !found
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
// are never loaded, so every selector through one is unresolved by design,
// and so is every member a local type promotes from an embedded import;
// those errors say nothing about the reviewed code.
func (pkg parsedPackage) localTypeError() string {
	var errs []error
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}}
	conf := types.Config{
		Importer: emptyImporter{},
		Error:    func(err error) { errs = append(errs, err) },
	}
	_, _ = conf.Check(pkg.name, pkg.fset, pkg.files, info)
	qualifiers := pkg.importQualifiers()
	promoted := pkg.importPromotedSelectors(info)
	for _, err := range errs {
		te, ok := err.(types.Error)
		if ok && (qualifiers.unresolved(te) || promoted[te.Pos] || strings.Contains(te.Msg, "imported and not used")) {
			continue
		}
		return err.Error()
	}
	return ""
}

// importPromotedSelectors is the position of every selector name whose
// receiver's members come in part from an unloaded import: a local type
// that embeds an imported type (struct{ sync.Mutex }) or is defined from
// one. Only loading the import could say whether the member exists (E-042).
func (pkg parsedPackage) importPromotedSelectors(info *types.Info) map[token.Pos]bool {
	out := map[token.Pos]bool{}
	for _, f := range pkg.files {
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if ok && hasImportedMembers(info.Types[sel.X].Type, map[types.Type]bool{}) {
				out[sel.Sel.Pos()] = true
			}
			return true
		})
	}
	return out
}

// hasImportedMembers reports whether t's method or field set includes
// members of a type no loaded package declares: an invalid (unloaded)
// type, or one embedded at any depth.
func hasImportedMembers(t types.Type, seen map[types.Type]bool) bool {
	if t == nil || seen[t] {
		return false
	}
	seen[t] = true
	if ptr, ok := t.(*types.Pointer); ok {
		return hasImportedMembers(ptr.Elem(), seen)
	}
	switch u := t.Underlying().(type) {
	case *types.Basic:
		return u.Kind() == types.Invalid
	case *types.Struct:
		for field := range u.Fields() {
			if field.Embedded() && hasImportedMembers(field.Type(), seen) {
				return true
			}
		}
	case *types.Interface:
		for embedded := range u.EmbeddedTypes() {
			if hasImportedMembers(embedded, seen) {
				return true
			}
		}
	}
	return false
}

// importQualifierSet is importQualifiers' result, by position.
type importQualifierSet map[token.Pos]qualifierKind

// qualifierKind says how sure importQualifiers is that a position is an
// import qualifier.
type qualifierKind int

const (
	// qualifierKnown is a name an import supplies for certain.
	qualifierKnown qualifierKind = iota + 1
	// qualifierIfUndefined is a selector base that is an import qualifier
	// exactly when no scope declares it.
	qualifierIfUndefined
)

// importQualifiers is the position of every package qualifier in the
// package: the X of a selector whose name is one of its file's imports.
// An unaliased import's package name is not always its last path element
// (gopkg.in/yaml.v3 is yaml, go-git/v5 is git), and imports are never
// loaded to learn it, so importNames guesses it from the path. Only in a
// file with an unaliased import none of whose guesses the file uses is
// every selector base a candidate qualifier; the type checker's
// "undefined" error then says whether one was (E-042).
func (pkg parsedPackage) importQualifiers() importQualifierSet {
	out := importQualifierSet{}
	for _, f := range pkg.files {
		names, unmatched := importNames(f)
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			id, ok := sel.X.(*ast.Ident)
			switch {
			case ok && names[id.Name]:
				out[id.Pos()] = qualifierKnown
				out[sel.Sel.Pos()] = qualifierKnown
			case ok && unmatched:
				out[id.Pos()] = qualifierIfUndefined
			}
			return true
		})
	}
	return out
}

// unresolved reports whether te is only an unloaded import's doing.
func (q importQualifierSet) unresolved(te types.Error) bool {
	switch q[te.Pos] {
	case qualifierKnown:
		return true
	case qualifierIfUndefined:
		return strings.HasPrefix(te.Msg, "undefined: ")
	default:
		return false
	}
}

// importNames is the set of names file f can qualify an import by: its
// alias, else every name importNameGuesses derives from its path, and evo
// for this module. unmatched reports whether an unaliased import has no
// guess the file uses as a selector base, so its real name is unknown.
func importNames(f *ast.File) (names map[string]bool, unmatched bool) {
	names = map[string]bool{}
	bases := selectorBases(f)
	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if imp.Name != nil {
			names[imp.Name.Name] = true
			continue
		}
		matched := false
		for _, guess := range importNameGuesses(path) {
			names[guess] = true
			matched = matched || bases[guess]
		}
		unmatched = unmatched || !matched
	}
	if name := evoImportName(f); name != "" {
		names[name] = true
	}
	return names, unmatched
}

// importNameGuesses are the package names Go convention derives from an
// import path: the last element, or the one before a /vN major version,
// with a .vN or .go suffix and a go- prefix or -go suffix removed.
func importNameGuesses(path string) []string {
	elems := strings.Split(path, "/")
	last := elems[len(elems)-1]
	if len(elems) > 1 && isMajorVersion(last) {
		last = elems[len(elems)-2]
	}
	base, _, _ := strings.Cut(last, ".")
	trimmed := strings.TrimSuffix(strings.TrimPrefix(base, "go-"), "-go")
	return []string{last, base, trimmed, strings.ReplaceAll(trimmed, "-", ""), strings.ReplaceAll(trimmed, "-", "_")}
}

// isMajorVersion reports whether elem is a module major-version path
// element such as v2 or v10.
func isMajorVersion(elem string) bool {
	digits, ok := strings.CutPrefix(elem, "v")
	return ok && digits != "" && strings.Trim(digits, "0123456789") == ""
}

// selectorBases is every identifier f uses as the X of a selector.
func selectorBases(f *ast.File) map[string]bool {
	out := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok {
				out[id.Name] = true
			}
		}
		return true
	})
	return out
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
