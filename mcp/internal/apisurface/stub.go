package apisurface

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// Stub renders the declaration-only source of the package in dir: every
// exported declaration, the unexported declarations those reach, and no
// function bodies, unexported struct fields or comments. Type-checking a
// program against it gives the same answers as type-checking against the
// package itself, without the package's source or its dependencies, so the
// reviewer can carry the 2.0 surface inside its own binary.
func Stub(dir string) ([]byte, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, fmt.Errorf("apisurface: list %s: %w", dir, err)
	}
	sort.Strings(paths)
	fset := token.NewFileSet()
	pkg := newStubPackage()
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, fmt.Errorf("apisurface: parse %s: %w", path, err)
		}
		pkg.add(file)
	}
	return pkg.render()
}

// stubPackage indexes the declarations of one package by the names they
// declare, so the closure of what an exported declaration reaches can be
// kept and the rest dropped.
type stubPackage struct {
	name    string
	imports map[string]string // import name -> path
	decls   []ast.Decl
	byName  map[string][]int // declared name -> indexes into decls
	methods map[string][]int // receiver type name -> indexes into decls
	kept    map[int]bool
	err     error
}

func newStubPackage() *stubPackage {
	return &stubPackage{imports: map[string]string{}, byName: map[string][]int{}, methods: map[string][]int{}, kept: map[int]bool{}}
}

func (p *stubPackage) add(file *ast.File) {
	p.name = file.Name.Name
	for _, imp := range file.Imports {
		path, _ := strconv.Unquote(imp.Path.Value)
		name := path[strings.LastIndex(path, "/")+1:]
		if imp.Name != nil {
			name = imp.Name.Name
		}
		p.imports[name] = path
	}
	for _, decl := range file.Decls {
		i := len(p.decls)
		switch d := decl.(type) {
		case *ast.FuncDecl:
			d.Doc, d.Body = nil, nil
			if d.Recv != nil && len(d.Recv.List) > 0 {
				recv := receiverName(d.Recv.List[0].Type)
				p.methods[recv] = append(p.methods[recv], i)
			} else {
				p.byName[d.Name.Name] = append(p.byName[d.Name.Name], i)
			}
		case *ast.GenDecl:
			if d.Tok == token.IMPORT {
				continue
			}
			d.Doc = nil
			for _, name := range genDeclNames(d) {
				p.byName[name] = append(p.byName[name], i)
			}
		default:
			continue
		}
		p.decls = append(p.decls, decl)
	}
}

func genDeclNames(d *ast.GenDecl) []string {
	var names []string
	for _, spec := range d.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			names = append(names, s.Name.Name)
		case *ast.ValueSpec:
			for _, n := range s.Names {
				names = append(names, n.Name)
			}
		}
	}
	return names
}

// keep marks decl i, trims it to its stub form, and keeps every package
// declaration it names, then the methods of each type it keeps.
func (p *stubPackage) keep(i int) {
	if p.kept[i] {
		return
	}
	p.kept[i] = true
	trimDecl(p.decls[i])
	p.dropForeignValues(p.decls[i])
	ast.Inspect(p.decls[i], func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			for _, j := range p.byName[id.Name] {
				p.keep(j)
			}
		}
		return true
	})
	if gen, ok := p.decls[i].(*ast.GenDecl); ok {
		for _, spec := range gen.Specs {
			if ts, ok := spec.(*ast.TypeSpec); ok {
				p.keepMethods(ts.Name.Name)
			}
		}
	}
}

// keepMethods keeps every method of typ. An unexported method keeps only
// its name: it exists so the type still satisfies an interface sealed by a
// method of that name, and its signature may name an internal package.
func (p *stubPackage) keepMethods(typ string) {
	for _, j := range p.methods[typ] {
		fd := p.decls[j].(*ast.FuncDecl)
		if !fd.Name.IsExported() && !p.kept[j] {
			fd.Type.Params, fd.Type.Results = &ast.FieldList{}, nil
		}
		p.keep(j)
	}
}

// dropForeignValues replaces the initializer of a value that reads another
// module's package (an Err* sentinel re-exported from an internal package)
// with its type: such a value is always an error. Any other such value has
// no type the stub can state, so the stub is refused.
func (p *stubPackage) dropForeignValues(decl ast.Decl) {
	gen, ok := decl.(*ast.GenDecl)
	if !ok {
		return
	}
	for _, spec := range gen.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok || vs.Type != nil || !p.readsForeignPackage(vs.Values) {
			continue
		}
		for _, name := range vs.Names {
			if !strings.HasPrefix(name.Name, "Err") {
				p.err = fmt.Errorf("apisurface: stub cannot state the type of %s, which reads another module's package", name.Name)
			}
		}
		vs.Type, vs.Values = ast.NewIdent("error"), nil
	}
}

func (p *stubPackage) readsForeignPackage(values []ast.Expr) bool {
	var foreign bool
	for _, v := range values {
		ast.Inspect(v, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok && strings.Contains(strings.SplitN(p.imports[id.Name], "/", 2)[0], ".") {
					foreign = true
				}
			}
			return true
		})
	}
	return foreign
}

// trimDecl drops what a stub never needs: unexported struct fields, tags,
// comments, and the signature of an unexported interface method (it only
// seals the interface; its parameter types may live in an internal
// package).
func trimDecl(decl ast.Decl) {
	ast.Inspect(decl, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.StructType:
			n.Fields.List = exportedFields(n.Fields.List)
		case *ast.InterfaceType:
			for _, m := range n.Methods.List {
				if ft, ok := m.Type.(*ast.FuncType); ok && len(m.Names) == 1 && !m.Names[0].IsExported() {
					ft.Params, ft.Results = &ast.FieldList{}, nil
				}
			}
		case *ast.Field:
			n.Doc, n.Comment, n.Tag = nil, nil, nil
		case *ast.ValueSpec:
			n.Doc, n.Comment = nil, nil
		case *ast.TypeSpec:
			n.Doc, n.Comment = nil, nil
		}
		return true
	})
}

func exportedFields(fields []*ast.Field) []*ast.Field {
	var out []*ast.Field
	for _, f := range fields {
		if len(f.Names) == 0 {
			if ast.IsExported(receiverName(f.Type)) {
				out = append(out, f)
			}
			continue
		}
		var names []*ast.Ident
		for _, n := range f.Names {
			if n.IsExported() {
				names = append(names, n)
			}
		}
		if len(names) > 0 {
			f.Names = names
			out = append(out, f)
		}
	}
	return out
}

func (p *stubPackage) render() ([]byte, error) {
	for i, decl := range p.decls {
		if declExported(decl) {
			p.keep(i)
		}
	}
	if p.err != nil {
		return nil, p.err
	}
	file := &ast.File{Name: ast.NewIdent(p.name)}
	used := map[string]bool{}
	for i, decl := range p.decls {
		if !p.kept[i] {
			continue
		}
		file.Decls = append(file.Decls, decl)
		ast.Inspect(decl, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok && p.imports[id.Name] != "" {
					used[id.Name] = true
				}
			}
			return true
		})
	}
	file.Decls = append([]ast.Decl{p.importDecl(used)}, file.Decls...)
	var buf bytes.Buffer
	buf.WriteString("// Code generated from the evident-output package declarations; DO NOT EDIT.\n\n")
	if err := printer.Fprint(&buf, token.NewFileSet(), file); err != nil {
		return nil, fmt.Errorf("apisurface: print stub: %w", err)
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

func (p *stubPackage) importDecl(used map[string]bool) ast.Decl {
	names := make([]string, 0, len(used))
	for name := range used {
		names = append(names, name)
	}
	sort.Strings(names)
	decl := &ast.GenDecl{Tok: token.IMPORT, Lparen: 1}
	for _, name := range names {
		path := p.imports[name]
		spec := &ast.ImportSpec{Path: &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(path)}}
		if path[strings.LastIndex(path, "/")+1:] != name {
			spec.Name = ast.NewIdent(name)
		}
		decl.Specs = append(decl.Specs, spec)
	}
	return decl
}

func declExported(decl ast.Decl) bool {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		return d.Recv == nil && d.Name.IsExported()
	case *ast.GenDecl:
		if slices.ContainsFunc(genDeclNames(d), ast.IsExported) {
			return true
		}
	}
	return false
}
