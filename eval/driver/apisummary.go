package driver

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"path"
	"strings"
)

// FixtureAPISummary renders the exported declarations of a fixture package
// godoc-style: doc comments and signatures, no function bodies.
func FixtureAPISummary(fixture fs.FS) (string, error) {
	names, err := fs.Glob(fixture, "*.go")
	if err != nil {
		return "", fmt.Errorf("list fixture files: %w", err)
	}
	fset := token.NewFileSet()
	var out strings.Builder
	for _, name := range names {
		data, err := fs.ReadFile(fixture, name)
		if err != nil {
			return "", fmt.Errorf("read fixture file %s: %w", name, err)
		}
		file, err := parser.ParseFile(fset, path.Base(name), data, parser.ParseComments)
		if err != nil {
			return "", fmt.Errorf("parse fixture file %s: %w", name, err)
		}
		if err := writeExported(&out, fset, file); err != nil {
			return "", err
		}
	}
	return out.String(), nil
}

func writeExported(out *strings.Builder, fset *token.FileSet, file *ast.File) error {
	if file.Doc != nil {
		fmt.Fprintf(out, "%s\n", file.Doc.Text())
	}
	for _, decl := range file.Decls {
		if !isExported(decl) {
			continue
		}
		if fn, ok := decl.(*ast.FuncDecl); ok {
			fn.Body = nil
		}
		writeDoc(out, declDoc(decl))
		var rendered bytes.Buffer
		if err := printer.Fprint(&rendered, fset, decl); err != nil {
			return fmt.Errorf("print fixture declaration: %w", err)
		}
		out.WriteString(rendered.String())
		out.WriteString("\n\n")
	}
	return nil
}

func declDoc(decl ast.Decl) *ast.CommentGroup {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		return d.Doc
	case *ast.GenDecl:
		return d.Doc
	}
	return nil
}

func writeDoc(out *strings.Builder, doc *ast.CommentGroup) {
	if doc == nil {
		return
	}
	for line := range strings.SplitSeq(strings.TrimRight(doc.Text(), "\n"), "\n") {
		out.WriteString("// " + line + "\n")
	}
}

// receiverExported reports whether a method's receiver type is exported;
// a plain function has no receiver and counts as exported.
func receiverExported(recv *ast.FieldList) bool {
	if recv == nil || len(recv.List) == 0 {
		return true
	}
	expr := recv.List[0].Type
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	ident, ok := expr.(*ast.Ident)
	return ok && ident.IsExported()
}

func isExported(decl ast.Decl) bool {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		return d.Name.IsExported() && receiverExported(d.Recv)
	case *ast.GenDecl:
		for _, spec := range d.Specs {
			switch s := spec.(type) {
			case *ast.TypeSpec:
				return s.Name.IsExported()
			case *ast.ValueSpec:
				return len(s.Names) > 0 && s.Names[0].IsExported()
			}
		}
	}
	return false
}
