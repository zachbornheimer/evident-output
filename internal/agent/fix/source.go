package fix

import (
	"go/ast"
	"go/token"
	"os"
	"strings"
	"sync"

	"golang.org/x/tools/go/analysis"
)

// fileCache memoizes on-disk file contents so every analyzer's source-text
// helper reads each analyzed file once, no matter how many findings it
// produces.
var fileCache sync.Map // filename -> []byte

func fileBytes(filename string) ([]byte, error) {
	if v, ok := fileCache.Load(filename); ok {
		return v.([]byte), nil
	}
	b, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	fileCache.Store(filename, b)
	return b, nil
}

// text returns node's exact source text. Analyzers use it to splice an
// argument or receiver expression into a rewritten call verbatim — never
// reformatted from the AST — so a multi-line argument or an inline comment
// inside it survives the fix unchanged.
func text(pass *analysis.Pass, node ast.Node) string {
	start := pass.Fset.Position(node.Pos())
	end := pass.Fset.Position(node.End())
	b, err := fileBytes(start.Filename)
	if err != nil || start.Offset > len(b) || end.Offset > len(b) || start.Offset > end.Offset {
		return ""
	}
	return string(b[start.Offset:end.Offset])
}

// argTexts renders every call argument's source text, in order.
func argTexts(pass *analysis.Pass, args []ast.Expr) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = text(pass, a)
	}
	return out
}

// hasVerbW reports whether a printf-style format argument is a string
// literal containing %w — the one case a rewritten return should still
// wrap an error instead of returning nil.
func hasVerbW(pass *analysis.Pass, format ast.Expr) bool {
	lit, ok := format.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return false
	}
	return strings.Contains(lit.Value, "%w")
}
