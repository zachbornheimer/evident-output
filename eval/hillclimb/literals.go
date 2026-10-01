package hillclimb

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"strings"
)

// StringLiteralsOnly reports whether two versions of a Go file differ only in
// string literal values and comments, so code behavior (scheduling included)
// cannot have changed.
func StringLiteralsOnly(before, after []byte) (bool, error) {
	left, err := canonicalWithoutStrings(before)
	if err != nil {
		return false, fmt.Errorf("compare original: %w", err)
	}
	right, err := canonicalWithoutStrings(after)
	if err != nil {
		return false, fmt.Errorf("compare edited: %w", err)
	}
	return left == right, nil
}

func canonicalWithoutStrings(src []byte) (string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "file.go", src, parser.SkipObjectResolution)
	if err != nil {
		return "", fmt.Errorf("parse Go source: %w", err)
	}
	ast.Inspect(file, func(node ast.Node) bool {
		if lit, ok := node.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			lit.Value = `""`
		}
		return true
	})
	var out bytes.Buffer
	if err := printer.Fprint(&out, fset, file); err != nil {
		return "", fmt.Errorf("print Go source: %w", err)
	}
	return strings.TrimSpace(out.String()), nil
}
