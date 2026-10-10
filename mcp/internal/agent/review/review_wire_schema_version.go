// Package review — EVO-WIRE-002 (C21-014): a wire struct whose schema
// version constant records no history. A single source cannot be diffed
// against its earlier revision, so the detectable signal is the convention
// the rule teaches: a file that declares a `...SchemaVersion` string
// constant next to json-tagged wire structs must document the constant, so a
// reviewer can see why the version moved when a property was added, renamed
// or removed. An undocumented version beside wire tags is where a breaking
// edit hides, and the finding asks for the record before the next edit.
package review

import (
	"go/ast"
	"go/token"
	"strings"
)

const schemaVersionSuffix = "SchemaVersion"

// detectUndocumentedWireSchemaVersion is EVO-WIRE-002.
func detectUndocumentedWireSchemaVersion(filename string, file *ast.File, fset *token.FileSet) []Finding {
	if !declaresJSONWireStruct(file) {
		return nil
	}
	var findings []Finding
	for _, spec := range schemaVersionConsts(file) {
		if spec.doc != nil && strings.TrimSpace(spec.doc.Text()) != "" {
			continue
		}
		pos := fset.Position(spec.name.Pos())
		findings = append(findings, Finding{
			RuleID:     "EVO-WIRE-002",
			File:       filename,
			Line:       pos.Line,
			Column:     pos.Column,
			Message:    spec.name.Name + " has no doc comment beside json-tagged wire structs: a renamed, removed or added property cannot be tied to a version bump",
			Suggestion: "document " + spec.name.Name + " with what changed in this version, and bump it in the same change that adds, renames or removes a json property",
		})
	}
	return findings
}

type schemaVersionConst struct {
	name *ast.Ident
	doc  *ast.CommentGroup
}

// schemaVersionConsts are the file's string constants named *SchemaVersion.
func schemaVersionConsts(file *ast.File) []schemaVersionConst {
	var out []schemaVersionConst
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs := spec.(*ast.ValueSpec)
			doc := vs.Doc
			if doc == nil && len(gen.Specs) == 1 {
				doc = gen.Doc
			}
			for i, name := range vs.Names {
				if strings.HasSuffix(name.Name, schemaVersionSuffix) && isStringLiteralAt(vs.Values, i) {
					out = append(out, schemaVersionConst{name: name, doc: doc})
				}
			}
		}
	}
	return out
}

func isStringLiteralAt(values []ast.Expr, i int) bool {
	if i >= len(values) {
		return false
	}
	lit, ok := values[i].(*ast.BasicLit)
	return ok && lit.Kind == token.STRING
}

// declaresJSONWireStruct reports whether any struct field carries a json tag.
func declaresJSONWireStruct(file *ast.File) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		field, ok := n.(*ast.Field)
		if ok && field.Tag != nil && strings.Contains(field.Tag.Value, `json:"`) {
			found = true
		}
		return !found
	})
	return found
}
