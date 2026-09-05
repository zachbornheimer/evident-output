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
//
// Files are grouped by directory before classification: the typed-value
// rule (see typedvalue.go) needs a package-wide index of evo-typed struct
// fields built from EVERY file in the directory before any single file's
// declarations can be classified, so a field declared in one file (e.g.
// app.go) is visible to a method using it in another (e.g. doctor.go).
func scanRepo(root string) ([]fileInventory, error) {
	byDir := map[string][]string{}
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
		dir := filepath.Dir(p)
		byDir[dir] = append(byDir[dir], p)
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("scan repo %s: %w", root, walkErr)
	}

	var files []fileInventory
	for _, paths := range byDir {
		dirFiles, err := scanDir(paths)
		if err != nil {
			return nil, fmt.Errorf("scan repo %s: %w", root, err)
		}
		files = append(files, dirFiles...)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

// parsedGoFile is one already-parsed source file, kept around between the
// index-building pass and the classification pass so each file is parsed
// exactly once.
type parsedGoFile struct {
	path        string
	fset        *token.FileSet
	file        *ast.File
	src         []byte
	idents      map[string]bool
	dotImported bool
}

// scanDir parses every file in one directory (Go package), builds that
// directory's typedValueIndex from all of them, then classifies each file's
// declarations against it.
func scanDir(paths []string) ([]fileInventory, error) {
	parsed := make([]parsedGoFile, 0, len(paths))
	for _, p := range paths {
		pf, err := parseGoFile(p)
		if err != nil {
			return nil, err
		}
		parsed = append(parsed, pf)
	}

	index := buildTypedValueIndex(parsed)

	var files []fileInventory
	for _, pf := range parsed {
		inv := fileInventory{Path: pf.path}
		for _, decl := range pf.file.Decls {
			u := classifyDecl(decl, pf.idents, pf.dotImported, index)
			if u == usageNone {
				continue
			}
			inv.Decls = append(inv.Decls, declFinding{
				Source: verbatimSource(pf.fset, pf.src, decl),
				Usage:  u,
			})
		}
		if len(inv.Decls) > 0 {
			files = append(files, inv)
		}
	}
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

// parseGoFile parses one Go file and resolves the evo identifiers its
// imports bind, without classifying any declaration — classification needs
// the whole directory's typedValueIndex first (see scanDir).
func parseGoFile(path string) (parsedGoFile, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return parsedGoFile{}, fmt.Errorf("read %s: %w", path, err)
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return parsedGoFile{}, fmt.Errorf("parse %s: %w", path, err)
	}
	idents, dotImported := evoIdentsInFile(f)
	return parsedGoFile{
		path:        path,
		fset:        fset,
		file:        f,
		src:         src,
		idents:      idents,
		dotImported: dotImported,
	}, nil
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
