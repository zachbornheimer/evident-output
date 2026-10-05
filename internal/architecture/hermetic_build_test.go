package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// noVCSStamp is the flag every test-run `go build` passes. A build that
// stamps VCS state fails outside a git work tree the current user owns: a
// source archive, the module cache, or a CI checkout owned by another
// user.
const noVCSStamp = "-buildvcs=false"

// TestTestBuildsDoNotStampVCS proves every test that builds a binary
// passes -buildvcs=false, so `go test ./...` passes from any copy of the
// module, not only an owned git checkout.
func TestTestBuildsDoNotStampVCS(t *testing.T) {
	root := moduleRoot(t)
	fset := token.NewFileSet()
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && path != root && (d.Name() == "testdata" || strings.HasPrefix(d.Name(), ".")) {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			return parseErr
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok && isGoBuild(call) && !hasStringArg(call, noVCSStamp) {
				t.Errorf("%s: `go build` without %s", fset.Position(call.Pos()), noVCSStamp)
			}
			return true
		})
		return nil
	})
	if walkErr != nil {
		t.Fatal(walkErr)
	}
}

// isGoBuild reports whether call's first two arguments are the literals
// "go" and "build", as in exec.Command("go", "build", ...).
func isGoBuild(call *ast.CallExpr) bool {
	return len(call.Args) >= 2 && stringLit(call.Args[0]) == "go" && stringLit(call.Args[1]) == "build"
}

// hasStringArg reports whether any of call's arguments is the literal want.
func hasStringArg(call *ast.CallExpr, want string) bool {
	for _, arg := range call.Args {
		if stringLit(arg) == want {
			return true
		}
	}
	return false
}

// stringLit is e's value when it is a string literal, else "".
func stringLit(e ast.Expr) string {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return ""
	}
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		return ""
	}
	return value
}
