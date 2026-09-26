package fix

import (
	"go/ast"
	"go/token"
	"os"
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

// enclosingFile returns the *ast.File among pass.Files that contains at,
// so a fix can inspect or edit that file's import list.
func enclosingFile(pass *analysis.Pass, at token.Pos) *ast.File {
	for _, f := range pass.Files {
		if f.Pos() <= at && at <= f.End() {
			return f
		}
	}
	return nil
}
