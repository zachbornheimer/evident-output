package adopt

import (
	"fmt"
	"go/ast"
	"go/token"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// facadeMigrationNote is templated with a facade's call-site count — see
// Facade.Note.
const facadeMigrationNote = "custom output facade: migrate the facade, not each call site — its %d call sites follow"

// mutationFacadeNote is templated with a mutation facade's call-site
// count — see Facade.Note and mutationVerbPrefixes.
const mutationFacadeNote = "custom mutation facade: migrate the facade, not each call site — its %d call sites follow"

// mutationVerbPrefixes names the method-name prefixes that mark a method as
// performing a real mutation — homelab's launchdfacade.Bootstrap,
// dockerfacade.ComposeUp/PullImage, filesystemfacade.WriteFile — rather
// than routing output through a writer. It is the one named, documented
// verb list both mutation-facade detection paths key off: a concrete
// type's declared method (mutationFacadeMethods) and a call through an
// interface-typed field or parameter with no method body at all
// (detectInterfaceFacades, ZYS-1019). "Pull" was added for ZYS-1019's
// acceptance fixture (deps.Docker.PullImage) — homelab's dockerfacade also
// calls this PullImage.
var mutationVerbPrefixes = []string{"Write", "Create", "Up", "Bootstrap", "Chmod", "Mkdir", "Delete", "Remove", "Pull"}

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
	facades = append(facades, detectInterfaceFacades(fset, files)...)
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

// mutationFacadeMethods reports the mutation-verb-named methods declared on
// a candidate type in ANY package — the *facade-suffixed package
// restriction was removed for ZYS-1019, since homelabctl's injected
// facades aren't required to live in a *facade package, only to carry
// mutation-verb method names.
func mutationFacadeMethods(c *facadeCandidate) []string {
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

// buildInterfaceTypeIndex returns the set of "dir.TypeName" keys for every
// interface declared across files — the type-resolution ZYS-1019 needs to
// tell a real, locally declared interface (Docker) from an external,
// concrete type (strings.Builder) it cannot chase into its declaration.
func buildInterfaceTypeIndex(files []parsedFile) map[string]bool {
	types := map[string]bool{}
	for _, pf := range files {
		dir := filepath.Dir(pf.Path)
		ast.Inspect(pf.File, func(n ast.Node) bool {
			spec, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			if _, ok := spec.Type.(*ast.InterfaceType); ok {
				types[dir+"."+spec.Name.Name] = true
			}
			return true
		})
	}
	return types
}

// interfaceFieldIndex maps "dir.StructTypeName" to its field names that are
// declared as one of interfaceTypes, to the interface type name that field
// carries — the resolution deps.Docker needs to know Docker (the field) is
// the Docker interface (the type), not just any field.
type interfaceFieldIndex map[string]map[string]string

func buildInterfaceFieldIndex(files []parsedFile, interfaceTypes map[string]bool) interfaceFieldIndex {
	idx := interfaceFieldIndex{}
	for _, pf := range files {
		dir := filepath.Dir(pf.Path)
		ast.Inspect(pf.File, func(n ast.Node) bool {
			spec, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			st, ok := spec.Type.(*ast.StructType)
			if !ok || st.Fields == nil {
				return true
			}
			key := dir + "." + spec.Name.Name
			for _, field := range st.Fields.List {
				name, ok := resolveNamedType(field.Type)
				if !ok || !interfaceTypes[dir+"."+name] {
					continue
				}
				for _, fieldName := range field.Names {
					if idx[key] == nil {
						idx[key] = map[string]string{}
					}
					idx[key][fieldName.Name] = name
				}
			}
			return true
		})
	}
	return idx
}

// detectInterfaceFacades is ZYS-1019's second mutation-facade path: a call
// through an interface-typed field (deps.Docker.PullImage) or an
// interface-typed parameter/receiver directly, matched on a mutation-verb
// method name (mutationVerbPrefixes), with no requirement that the
// interface's implementing type even be visible — Docker here never gets a
// method body, only a declaration. Matching is restricted to receivers
// resolved to a locally declared interface, so a concrete external type
// like strings.Builder (WriteString) is never mistaken for a facade.
func detectInterfaceFacades(fset *token.FileSet, files []parsedFile) []Facade {
	interfaceTypes := buildInterfaceTypeIndex(files)
	if len(interfaceTypes) == 0 {
		return nil
	}
	fields := buildInterfaceFieldIndex(files, interfaceTypes)

	byInterface := map[string]*Facade{}
	for _, pf := range files {
		dir := filepath.Dir(pf.Path)
		ast.Inspect(pf.File, func(n ast.Node) bool {
			decl, ok := n.(*ast.FuncDecl)
			if !ok || decl.Body == nil {
				return true
			}
			bindings := bindingsFor(decl, dir, interfaceTypes)
			ast.Inspect(decl.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || !isMutationVerbMethod(sel.Sel.Name) {
					return true
				}
				ifaceType, ok := resolveInterfaceReceiver(sel.X, dir, bindings, fields)
				if !ok {
					return true
				}
				f, ok := byInterface[ifaceType]
				if !ok {
					f = &Facade{Type: ifaceType, File: pf.Path, isMutation: true}
					byInterface[ifaceType] = f
				}
				if !containsString(f.Methods, sel.Sel.Name) {
					f.Methods = append(f.Methods, sel.Sel.Name)
				}
				pos := fset.Position(call.Pos())
				f.CallSites = append(f.CallSites, fmt.Sprintf("%s:%d", pf.Path, pos.Line))
				return true
			})
			return true
		})
	}

	var facades []Facade
	for _, f := range byInterface {
		sort.Strings(f.Methods)
		sort.Strings(f.CallSites)
		f.Note = fmt.Sprintf(mutationFacadeNote, len(f.CallSites))
		facades = append(facades, *f)
	}
	return facades
}

// resolveInterfaceReceiver resolves the interface type name a mutation-verb
// call's receiver carries: a bare identifier bound directly to an
// interface-typed parameter/receiver, or a struct-field access whose field
// is declared as an interface type. Anything it can't resolve syntactically
// (an external package's concrete type, an unbound local variable) reports
// ok=false rather than guessing.
func resolveInterfaceReceiver(expr ast.Expr, dir string, bindings map[string]identBinding, fields interfaceFieldIndex) (string, bool) {
	switch e := expr.(type) {
	case *ast.Ident:
		binding, ok := bindings[e.Name]
		if ok && binding.interfaceType != "" {
			return binding.interfaceType, true
		}
		return "", false
	case *ast.SelectorExpr:
		base, ok := e.X.(*ast.Ident)
		if !ok {
			return "", false
		}
		binding, ok := bindings[base.Name]
		if !ok || binding.typeName == "" {
			return "", false
		}
		if name, ok := fields[dir+"."+binding.typeName][e.Sel.Name]; ok {
			return name, true
		}
		return "", false
	default:
		return "", false
	}
}

func containsString(list []string, s string) bool {
	return slices.Contains(list, s)
}
