package main

import (
	"go/ast"
	"go/token"
	"path"
	"strings"
)

// evoIdentsInFile returns the local identifiers a file's imports bind to
// evoModulePath or one of its subpackages (honoring import aliases), and
// whether evo is dot-imported. A blank import ("_") binds no usable
// identifier and is excluded.
func evoIdentsInFile(f *ast.File) (idents map[string]bool, dotImported bool) {
	idents = map[string]bool{}
	for _, imp := range f.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		if importPath != evoModulePath && !strings.HasPrefix(importPath, evoModulePath+"/") {
			continue
		}
		switch {
		case imp.Name == nil:
			idents[path.Base(importPath)] = true
		case imp.Name.Name == "_":
			// no usable identifier
		case imp.Name.Name == ".":
			dotImported = true
		default:
			idents[imp.Name.Name] = true
		}
	}
	return idents, dotImported
}

// referencesEvoIdent reports whether node contains a selector expression
// (evo.Foo, in either an expression or a type position — both parse to the
// same *ast.SelectorExpr node) rooted at one of idents. node may be nil.
func referencesEvoIdent(node ast.Node, idents map[string]bool) bool {
	if node == nil || len(idents) == 0 {
		return false
	}
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		if found {
			return false
		}
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); ok && idents[id.Name] {
			found = true
			return false
		}
		return true
	})
	return found
}

// classifyFuncDecl applies the three-tier rule to one function or method,
// in precedence order: direct if its body calls through an evo identifier;
// else evo-typed if its receiver, params, or results reference an evo type;
// else evo-typed value if its body reaches evo only through a receiver or
// parameter field resolved via index (see classifyTypedValue).
func classifyFuncDecl(fd *ast.FuncDecl, idents map[string]bool, index typedValueIndex) usage {
	if fd.Body != nil && referencesEvoIdent(fd.Body, idents) {
		return usageDirect
	}
	if fd.Recv != nil && referencesEvoIdent(fd.Recv, idents) {
		return usageEvoTyped
	}
	if fd.Type.Params != nil && referencesEvoIdent(fd.Type.Params, idents) {
		return usageEvoTyped
	}
	if fd.Type.Results != nil && referencesEvoIdent(fd.Type.Results, idents) {
		return usageEvoTyped
	}
	return classifyTypedValue(fd, index)
}

// classifyGenDecl applies the grouped-declaration rule from the work order:
// a var/const/type block is direct if any spec inside it is direct (a
// value expression calling through an evo identifier), else evo-typed if
// any spec's declared type references an evo type. Import blocks never
// qualify.
func classifyGenDecl(gd *ast.GenDecl, idents map[string]bool) usage {
	best := usageNone
	for _, spec := range gd.Specs {
		switch s := spec.(type) {
		case *ast.ValueSpec:
			for _, v := range s.Values {
				if referencesEvoIdent(v, idents) {
					return usageDirect
				}
			}
			if s.Type != nil && referencesEvoIdent(s.Type, idents) {
				best = usageEvoTyped
			}
		case *ast.TypeSpec:
			if referencesEvoIdent(s.Type, idents) {
				best = usageEvoTyped
			}
		}
	}
	return best
}

// classifyFile marks every file dot-importing evo conservatively as direct
// on every declaration — a dot import erases the qualifier a static
// classifier relies on, so "can't tell" resolves to "include it".
func classifyDecl(decl ast.Decl, idents map[string]bool, dotImported bool, index typedValueIndex) usage {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		if dotImported {
			return usageDirect
		}
		return classifyFuncDecl(d, idents, index)
	case *ast.GenDecl:
		if d.Tok == token.IMPORT {
			return usageNone
		}
		if dotImported {
			return usageDirect
		}
		return classifyGenDecl(d, idents)
	default:
		return usageNone
	}
}
