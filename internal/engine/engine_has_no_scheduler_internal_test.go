package engine

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// concurrencyExemptions are the engine files that may start a goroutine or
// use a channel, each for a reason that is not deciding when work runs. The
// scheduler (what is eligible, what starts, what a wait releases) is the
// graph's alone; anything else in engine that spawns or orders by channel
// fails TestEngineHoldsNoSchedulingDecision.
var concurrencyExemptions = map[string]string{
	"confirm.go":          "stdin read for the prompt: terminal interaction, moves with slice 6/8",
	"run.go":              "signal-interruptible run: composition root, slice 8",
	"run_signal.go":       "signal delivery facade types of run.go (chan<- Signal): composition root, slice 8",
	"output.go":           "the closing latch field: lifecycle, slice 8",
	"output_lifecycle.go": "the closing latch: lifecycle, slice 8",
	"live.go":             "spinner animation loop: project/live, slice 6",
}

// TestEngineHoldsNoSchedulingDecision pins that no engine file declares a
// goroutine, a sync.WaitGroup or a channel except the exemptions above. The
// graph owns the scheduler wholly; a worker pool, a drain or a wake-up
// signal growing back here would be a second scheduler.
func TestEngineHoldsNoSchedulingDecision(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("list engine sources: %v", err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		if _, exempt := concurrencyExemptions[name]; exempt {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		file, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if what := orderingConstruct(n); what != "" {
				t.Errorf("%s: %s; scheduling belongs to internal/graph (exemptions: concurrencyExemptions)",
					fset.Position(n.Pos()), what)
			}
			return true
		})
	}
}

// orderingConstruct names the goroutine, wait group or channel n declares,
// or "" when n is none of them.
func orderingConstruct(n ast.Node) string {
	switch n := n.(type) {
	case *ast.GoStmt:
		return "go statement"
	case *ast.ChanType:
		return "channel type"
	case *ast.SelectorExpr:
		if pkg, ok := n.X.(*ast.Ident); ok && pkg.Name == "sync" && n.Sel.Name == "WaitGroup" {
			return "sync.WaitGroup"
		}
	}
	return ""
}
