package apisurface

import (
	"fmt"
	"go/ast"
	"go/doc"
	"go/token"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/zachbornheimer/evident-output/internal/modpin"
)

// aliasResolver finds the declaration behind a root-package alias to an
// in-module type, so Walk can list the members the alias exposes. Each
// target package is parsed once.
type aliasResolver struct {
	fset     *token.FileSet
	root     string
	imports  map[string]string // local import name → import path
	packages map[string]*doc.Package
}

func newAliasResolver(fset *token.FileSet, root string, files []*ast.File) *aliasResolver {
	imports := map[string]string{}
	for _, f := range files {
		for _, spec := range f.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				continue
			}
			name := path.Base(importPath)
			if spec.Name != nil {
				name = spec.Name.Name
			}
			imports[name] = importPath
		}
	}
	return &aliasResolver{fset: fset, root: root, imports: imports, packages: map[string]*doc.Package{}}
}

// target returns the in-module type typ aliases, or nil when typ is not an
// alias of the form pkg.Name into this module (a declared type, or an
// alias to the standard library, which is not this module's API).
func (r *aliasResolver) target(typ *doc.Type) (*doc.Type, error) {
	pkgName, typeName, ok := aliasSelector(typ)
	if !ok {
		return nil, nil
	}
	importPath, ok := r.imports[pkgName]
	rel, inModule := strings.CutPrefix(importPath, modpin.ModulePath+"/")
	if !ok || !inModule {
		return nil, nil
	}
	pkg, err := r.load(importPath, filepath.Join(r.root, filepath.FromSlash(rel)))
	if err != nil {
		return nil, err
	}
	for _, t := range pkg.Types {
		if t.Name == typeName {
			return t, nil
		}
	}
	return nil, fmt.Errorf("apisurface: alias %s names %s.%s, which %s does not declare", typ.Name, pkgName, typeName, importPath)
}

func (r *aliasResolver) load(importPath, dir string) (*doc.Package, error) {
	if pkg, ok := r.packages[importPath]; ok {
		return pkg, nil
	}
	files, err := parseDir(r.fset, dir)
	if err != nil {
		return nil, err
	}
	pkg, err := doc.NewFromFiles(r.fset, files, importPath)
	if err != nil {
		return nil, fmt.Errorf("apisurface: doc %s: %w", dir, err)
	}
	r.packages[importPath] = pkg
	return pkg, nil
}

// aliasSelector reports the pkg and Name of an alias declared as
// type T = pkg.Name.
func aliasSelector(typ *doc.Type) (pkgName, typeName string, ok bool) {
	for _, spec := range typ.Decl.Specs {
		ts, isType := spec.(*ast.TypeSpec)
		if !isType || ts.Name.Name != typ.Name || !ts.Assign.IsValid() {
			continue
		}
		sel, isSel := ts.Type.(*ast.SelectorExpr)
		if !isSel {
			return "", "", false
		}
		pkg, isIdent := sel.X.(*ast.Ident)
		if !isIdent {
			return "", "", false
		}
		return pkg.Name, sel.Sel.Name, true
	}
	return "", "", false
}
