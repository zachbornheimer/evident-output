package apisurface

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
)

// DeclFiles maps every exported identifier declared in the non-test Go files
// of dir to the base name of the file that declares it. Keys use the Ident
// format: "Foo" for a type, value or function, "Recv.Name" for a method.
func DeclFiles(dir string) (map[string]string, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, fmt.Errorf("apisurface: list %s: %w", dir, err)
	}
	fset := token.NewFileSet()
	files := make(map[string]string)
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, fmt.Errorf("apisurface: parse %s: %w", path, err)
		}
		base := filepath.Base(path)
		for _, decl := range file.Decls {
			for _, name := range declIdents(decl) {
				files[name] = base
			}
		}
	}
	return files, nil
}

func declIdents(decl ast.Decl) []string {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		if !d.Name.IsExported() {
			return nil
		}
		if d.Recv == nil || len(d.Recv.List) == 0 {
			return []string{d.Name.Name}
		}
		return []string{receiverName(d.Recv.List[0].Type) + "." + d.Name.Name}
	case *ast.GenDecl:
		var names []string
		for _, spec := range d.Specs {
			switch s := spec.(type) {
			case *ast.TypeSpec:
				names = appendExported(names, s.Name)
			case *ast.ValueSpec:
				names = appendExported(names, s.Names...)
			}
		}
		return names
	}
	return nil
}

func appendExported(names []string, idents ...*ast.Ident) []string {
	for _, id := range idents {
		if id.IsExported() {
			names = append(names, id.Name)
		}
	}
	return names
}

// receiverName strips pointers and type parameters: *Foo[T] becomes Foo.
func receiverName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return receiverName(t.X)
	case *ast.IndexExpr:
		return receiverName(t.X)
	case *ast.IndexListExpr:
		return receiverName(t.X)
	case *ast.ParenExpr:
		return receiverName(t.X)
	case *ast.Ident:
		return t.Name
	}
	return ""
}
