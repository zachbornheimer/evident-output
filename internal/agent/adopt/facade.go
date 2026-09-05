package adopt

import (
	"fmt"
	"go/ast"
	"go/token"
	"path/filepath"
	"sort"
)

// facadeMigrationNote is templated with a facade's call-site count — see
// Facade.Note.
const facadeMigrationNote = "custom output facade: migrate the facade, not each call site — its %d call sites follow"

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
		ast.Inspect(pf.File, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.TypeSpec:
				recordWriterFields(candidates, dir, pf.Path, node)
			case *ast.FuncDecl:
				recordMethodBody(candidates, dir, pf.Path, node)
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

// confirmedFacades keeps only candidates with at least one method whose
// body actually wraps a writer field — a struct merely holding an
// io.Writer field isn't a facade until something writes through it.
func confirmedFacades(candidates map[string]*facadeCandidate) []Facade {
	var facades []Facade
	for _, c := range candidates {
		var methods []string
		for name, body := range c.methodBodies {
			if wrapsWriter(body, c.writerFields) {
				methods = append(methods, name)
			}
		}
		if len(methods) == 0 {
			continue
		}
		sort.Strings(methods)
		facades = append(facades, Facade{Type: c.typeName, File: c.file, Methods: methods})
	}
	return facades
}

func recordWriterFields(candidates map[string]*facadeCandidate, dir, path string, spec *ast.TypeSpec) {
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
	c := candidateFor(candidates, dir, path, spec.Name.Name)
	for _, f := range fields {
		c.writerFields[f] = true
	}
}

func recordMethodBody(candidates map[string]*facadeCandidate, dir, path string, decl *ast.FuncDecl) {
	if decl.Recv == nil || len(decl.Recv.List) == 0 || decl.Body == nil {
		return
	}
	typeName := receiverTypeName(decl.Recv.List[0].Type)
	if typeName == "" {
		return
	}
	c := candidateFor(candidates, dir, path, typeName)
	c.methodBodies[decl.Name.Name] = decl.Body
}

func candidateFor(candidates map[string]*facadeCandidate, dir, path, typeName string) *facadeCandidate {
	key := dir + "." + typeName
	c, ok := candidates[key]
	if !ok {
		c = &facadeCandidate{
			typeName:     typeName,
			file:         path,
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
	if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "os" &&
		(sel.Sel.Name == "Stdout" || sel.Sel.Name == "Stderr") {
		return true
	}
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
		facades[i].Note = fmt.Sprintf(facadeMigrationNote, len(facades[i].CallSites))
	}
}
