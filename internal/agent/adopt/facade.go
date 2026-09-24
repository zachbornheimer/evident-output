package adopt

import (
	"fmt"
	"go/ast"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
)

// facadeMigrationNote is templated with a facade's call-site count — see
// Facade.Note.
const facadeMigrationNote = "custom output facade: migrate the facade, not each call site — its %d call sites follow"

// mutationFacadeNote is templated with a mutation facade's call-site
// count — see Facade.Note and mutationVerbPrefixes.
const mutationFacadeNote = "custom mutation facade: migrate the facade, not each call site — its %d call sites follow"

// mutationVerbPrefixes names the method-name prefixes that mark a *facade
// package's type as performing a real mutation (launchdfacade.Bootstrap,
// dockerfacade.ComposeUp, filesystemfacade.WriteFile) rather than routing
// output through a writer. Detection has no io.Writer field to key off —
// homelab's launchdfacade.CLI holds none — so the naming convention plus
// the *facade package convention (facadePackage) together stand in for the
// type resolution adopt doesn't do (see ZYS-1019).
var mutationVerbPrefixes = []string{"Write", "Up", "Bootstrap"}

// isMutationVerbMethod reports whether name matches one of
// mutationVerbPrefixes.
func isMutationVerbMethod(name string) bool {
	for _, prefix := range mutationVerbPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// isFacadePackage reports whether pkgName follows the *facade naming
// convention homelab's dockerfacade/colimafacade/launchdfacade/
// filesystemfacade packages use — the signal that stands in for an
// io.Writer field when a candidate's methods are pure mutations.
func isFacadePackage(pkgName string) bool {
	return strings.HasSuffix(strings.ToLower(pkgName), "facade")
}

// facadeInventoryCaveat discloses that facade call-site enumeration is a
// selector-name heuristic, not full type resolution — see Plan.Caveat.
const facadeInventoryCaveat = "output may flow through unclassified writers — inventory is a floor, not a census"

// parsedFile pairs one already-parsed source file with the path it came
// from, so detectFacades can make a second pass over the exact ASTs
// inventoryFile already parsed without re-reading or re-parsing anything.
type parsedFile struct {
	Path string
	File *ast.File
}

// facadeCandidate accumulates one (package directory, type name) pair's
// io.Writer fields and method bodies as files are visited — a struct's
// field declaration and its methods can live in different files of the
// same package, so nothing about a type can be judged file-by-file.
type facadeCandidate struct {
	typeName     string
	file         string
	pkgName      string
	writerFields map[string]bool
	methodBodies map[string]*ast.BlockStmt
}

// detectFacades finds every custom output-facade type across files and
// enumerates each one's call sites. Both detection and counting are
// selector-name heuristics, not full type resolution — that's what
// facadeInventoryCaveat discloses on the Plan.
func detectFacades(fset *token.FileSet, files []parsedFile) []Facade {
	candidates := map[string]*facadeCandidate{}
	for _, pf := range files {
		dir := filepath.Dir(pf.Path)
		pkgName := pf.File.Name.Name
		ast.Inspect(pf.File, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.TypeSpec:
				recordWriterFields(candidates, dir, pf.Path, pkgName, node)
			case *ast.FuncDecl:
				recordMethodBody(candidates, dir, pf.Path, pkgName, node)
			}
			return true
		})
	}

	facades := confirmedFacades(candidates)
	enumerateCallSites(fset, files, facades)
	sort.Slice(facades, func(i, j int) bool {
		if facades[i].File != facades[j].File {
			return facades[i].File < facades[j].File
		}
		return facades[i].Type < facades[j].Type
	})
	return facades
}

// confirmedFacades keeps only candidates confirmed one of two ways: an
// output facade (at least one method whose body actually wraps a writer
// field — a struct merely holding an io.Writer field isn't a facade until
// something writes through it), or a mutation facade (a *facade-package
// type with at least one Write*/Up*/Bootstrap*-named method — see
// mutationVerbPrefixes for why homelab's launchdfacade/dockerfacade/
// filesystemfacade need this second path instead of a writer field).
func confirmedFacades(candidates map[string]*facadeCandidate) []Facade {
	var facades []Facade
	for _, c := range candidates {
		if methods := outputFacadeMethods(c); len(methods) > 0 {
			facades = append(facades, Facade{Type: c.typeName, File: c.file, Methods: methods, isMutation: false})
			continue
		}
		if methods := mutationFacadeMethods(c); len(methods) > 0 {
			facades = append(facades, Facade{Type: c.typeName, File: c.file, Methods: methods, isMutation: true})
		}
	}
	return facades
}

func outputFacadeMethods(c *facadeCandidate) []string {
	if len(c.writerFields) == 0 {
		return nil
	}
	var methods []string
	for name, body := range c.methodBodies {
		if wrapsWriter(body, c.writerFields) {
			methods = append(methods, name)
		}
	}
	sort.Strings(methods)
	return methods
}

func mutationFacadeMethods(c *facadeCandidate) []string {
	if !isFacadePackage(c.pkgName) {
		return nil
	}
	var methods []string
	for name := range c.methodBodies {
		if isMutationVerbMethod(name) {
			methods = append(methods, name)
		}
	}
	sort.Strings(methods)
	return methods
}

func recordWriterFields(candidates map[string]*facadeCandidate, dir, path, pkgName string, spec *ast.TypeSpec) {
	st, ok := spec.Type.(*ast.StructType)
	if !ok || st.Fields == nil {
		return
	}
	var fields []string
	for _, field := range st.Fields.List {
		if !isIOWriterType(field.Type) {
			continue
		}
		for _, name := range field.Names {
			fields = append(fields, name.Name)
		}
	}
	if len(fields) == 0 {
		return
	}
	c := candidateFor(candidates, dir, path, pkgName, spec.Name.Name)
	for _, f := range fields {
		c.writerFields[f] = true
	}
}

func recordMethodBody(candidates map[string]*facadeCandidate, dir, path, pkgName string, decl *ast.FuncDecl) {
	if decl.Recv == nil || len(decl.Recv.List) == 0 || decl.Body == nil {
		return
	}
	typeName := receiverTypeName(decl.Recv.List[0].Type)
	if typeName == "" {
		return
	}
	c := candidateFor(candidates, dir, path, pkgName, typeName)
	c.methodBodies[decl.Name.Name] = decl.Body
}

func candidateFor(candidates map[string]*facadeCandidate, dir, path, pkgName, typeName string) *facadeCandidate {
	key := dir + "." + typeName
	c, ok := candidates[key]
	if !ok {
		c = &facadeCandidate{
			typeName:     typeName,
			file:         path,
			pkgName:      pkgName,
			writerFields: map[string]bool{},
			methodBodies: map[string]*ast.BlockStmt{},
		}
		candidates[key] = c
	}
	return c
}

func receiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return receiverTypeName(t.X)
	default:
		return ""
	}
}

func isIOWriterType(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "io" && sel.Sel.Name == "Writer"
}

// wrapsWriter reports whether body writes through one of writerFields —
// either directly via fmt.Fprint*, or indirectly via a color-printer
// closure (color.New(...).FprintfFunc(), the shape go-task's
// internal/logger uses and a bare fmt/os call-site classifier can't see).
func wrapsWriter(body *ast.BlockStmt, writerFields map[string]bool) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if isColorPrinterFunc(call) || isFmtFprintToWriter(call, writerFields) {
				found = true
			}
		}
		return true
	})
	return found
}

func isColorPrinterFunc(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	switch sel.Sel.Name {
	case "FprintfFunc", "FprintlnFunc", "FprintFunc":
		return true
	default:
		return false
	}
}

func isFmtFprintToWriter(call *ast.CallExpr, writerFields map[string]bool) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || pkg.Name != "fmt" {
		return false
	}
	switch sel.Sel.Name {
	case "Fprint", "Fprintf", "Fprintln":
	default:
		return false
	}
	return len(call.Args) > 0 && targetsWriter(call.Args[0], writerFields)
}

func targetsWriter(arg ast.Expr, writerFields map[string]bool) bool {
	sel, ok := arg.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	// os.Stdout/os.Stderr only count when they are this type's writer
	// fields (l.Stdout) — a method that fmt.Fprintf(os.Stderr) without
	// an io.Writer field is not a facade.
	return writerFields[sel.Sel.Name]
}

// enumerateCallSites walks every file's call expressions and, for each
// selector call whose method name matches a facade's, records "path:line".
// Matching is by method name alone, not receiver type resolution — two
// unrelated types sharing a method name in the same tree would both
// collect the call, which is exactly what facadeInventoryCaveat discloses.
func enumerateCallSites(fset *token.FileSet, files []parsedFile, facades []Facade) {
	byMethod := map[string][]*Facade{}
	for i := range facades {
		for _, m := range facades[i].Methods {
			byMethod[m] = append(byMethod[m], &facades[i])
		}
	}
	if len(byMethod) == 0 {
		return
	}
	for _, pf := range files {
		ast.Inspect(pf.File, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			for _, f := range byMethod[sel.Sel.Name] {
				pos := fset.Position(call.Pos())
				f.CallSites = append(f.CallSites, fmt.Sprintf("%s:%d", pf.Path, pos.Line))
			}
			return true
		})
	}
	for i := range facades {
		sort.Strings(facades[i].CallSites)
		note := facadeMigrationNote
		if facades[i].isMutation {
			note = mutationFacadeNote
		}
		facades[i].Note = fmt.Sprintf(note, len(facades[i].CallSites))
	}
}
