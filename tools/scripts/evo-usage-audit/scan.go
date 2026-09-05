package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// scanRepo walks root for .go files and returns the evo usage inventory for
// every file that has at least one qualifying declaration, sorted by
// absolute path. It skips vendor/, .git/, and any directory whose name
// starts with "." or "_".
func scanRepo(root string) ([]fileInventory, error) {
	var files []fileInventory
	walkErr := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", p, err)
		}
		if d.IsDir() {
			return skipDirErr(d.Name())
		}
		if !strings.HasSuffix(p, ".go") {
			return nil
		}
		inv, err := scanFile(p)
		if err != nil {
			return fmt.Errorf("scan %s: %w", p, err)
		}
		if len(inv.Decls) > 0 {
			files = append(files, inv)
		}
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("scan repo %s: %w", root, walkErr)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

// skipDirErr tells filepath.WalkDir to skip a directory this tool never
// inventories: vendor/, .git/, and any dot- or underscore-prefixed name
// (root "." itself is exempt from the dot rule).
func skipDirErr(name string) error {
	skip := name == "vendor" || name == ".git" ||
		(name != "." && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")))
	if skip {
		return filepath.SkipDir
	}
	return nil
}

// scanFile parses one Go file and returns its qualifying declarations in
// source order, each rendered as verbatim source (doc comment included).
func scanFile(path string) (fileInventory, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return fileInventory{}, fmt.Errorf("read %s: %w", path, err)
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return fileInventory{}, fmt.Errorf("parse %s: %w", path, err)
	}

	idents, dotImported := evoIdentsInFile(f)
	inv := fileInventory{Path: path}
	for _, decl := range f.Decls {
		u := classifyDecl(decl, idents, dotImported)
		if u == usageNone {
			continue
		}
		inv.Decls = append(inv.Decls, declFinding{
			Source: verbatimSource(fset, src, decl),
			Usage:  u,
		})
	}
	return inv, nil
}

// verbatimSource slices the original file bytes covering decl, doc comment
// included when present, so the rendered block is exactly what's on disk —
// never a go/printer reformat.
func verbatimSource(fset *token.FileSet, src []byte, decl ast.Decl) string {
	start := decl.Pos()
	if doc := declDoc(decl); doc != nil {
		start = doc.Pos()
	}
	startOffset := fset.Position(start).Offset
	endOffset := fset.Position(decl.End()).Offset
	return string(src[startOffset:endOffset])
}

func declDoc(decl ast.Decl) *ast.CommentGroup {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		return d.Doc
	case *ast.GenDecl:
		return d.Doc
	default:
		return nil
	}
}
