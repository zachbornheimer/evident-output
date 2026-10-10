package engine

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The constructs by which engine code could decide when work runs: start a
// goroutine, wait on a group of them, or order by channel or timer.
const (
	goStatement   = "go statement"
	channelType   = "channel type"
	waitGroup     = "sync.WaitGroup"
	selectStmt    = "select statement"
	sendStmt      = "channel send"
	receiveExpr   = "channel receive"
	afterFuncCall = "AfterFunc call"
)

// concurrencyExemption lets one engine file use some scheduling constructs,
// for a reason that is not deciding when work runs. constructs lists exactly
// what is allowed; nil allows every construct, for files that are wholly the
// lifecycle or terminal composition root.
type concurrencyExemption struct {
	constructs []string
	reason     string
}

func (e concurrencyExemption) allows(construct string) bool {
	if e.constructs == nil {
		return true
	}
	return slices.Contains(e.constructs, construct)
}

// concurrencyExemptions are the engine files that may use a scheduling
// construct. The scheduler (what is eligible, what starts, what a wait
// releases) is the graph's alone; anything else in engine that spawns or
// orders by channel or timer fails TestEngineHoldsNoSchedulingDecision.
var concurrencyExemptions = map[string]concurrencyExemption{
	"confirm.go":          {reason: "stdin read for the prompt: terminal interaction, moves with slice 6/8"},
	"run.go":              {reason: "signal-interruptible run: composition root, slice 8"},
	"output.go":           {reason: "the closing latch field: lifecycle, slice 8"},
	"output_lifecycle.go": {reason: "the closing latch: lifecycle, slice 8"},
	"live.go":             {reason: "spinner animation loop: project/live, slice 6"},
	"run_signal.go": {
		constructs: []string{channelType},
		reason:     "signal delivery facade types of run.go (chan<- Signal): composition root, slice 8",
	},
	"plain_heartbeat.go": {
		constructs: []string{afterFuncCall},
		reason:     "the plain heartbeat timer re-arms through the Scheduler facade: project/live, slice 6",
	},
}

// TestEngineHoldsNoSchedulingDecision pins that no engine file starts a
// goroutine, waits on a sync.WaitGroup, or orders by channel (declare, send,
// receive, select) or timer (AfterFunc) except the exemptions above. The
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
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		exemption, exempt := concurrencyExemptions[name]
		for _, use := range schedulingConstructsIn(t, fset, name, src) {
			if exempt && exemption.allows(use.construct) {
				continue
			}
			t.Errorf("%s: %s; scheduling belongs to internal/graph (exemptions: concurrencyExemptions)",
				use.position, use.construct)
		}
	}
}

// Each construct the guard names must be seen where it appears, including the
// ones that look harmless in isolation: a bare receive on ctx.Done() is a
// wait, and a select is an ordering decision.
func TestEngineGuardNamesEverySchedulingConstruct(t *testing.T) {
	cases := map[string]struct {
		body string
		want string
	}{
		"go statement":     {"func f() { go f() }", goStatement},
		"channel type":     {"var c chan int", channelType},
		"wait group":       {"var w sync.WaitGroup", waitGroup},
		"select":           {"func f(c chan int) { select { default: } }", selectStmt},
		"send":             {"func f(c chan int) { c <- 1 }", sendStmt},
		"receive":          {"func f(ctx context.Context) { <-ctx.Done() }", receiveExpr},
		"time.AfterFunc":   {"func f() { time.AfterFunc(0, f) }", afterFuncCall},
		"facade AfterFunc": {"func f(s interface{ AfterFunc() }) { s.AfterFunc() }", afterFuncCall},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			src := "package engine\n" + c.body
			found := false
			for _, use := range schedulingConstructsIn(t, token.NewFileSet(), "probe.go", []byte(src)) {
				found = found || use.construct == c.want
			}
			if !found {
				t.Errorf("guard missed %q in %q", c.want, c.body)
			}
		})
	}
}

// schedulingUse is one scheduling construct found in a source file.
type schedulingUse struct {
	position  token.Position
	construct string
}

func schedulingConstructsIn(t *testing.T, fset *token.FileSet, name string, src []byte) []schedulingUse {
	t.Helper()
	file, err := parser.ParseFile(fset, name, src, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	var uses []schedulingUse
	ast.Inspect(file, func(n ast.Node) bool {
		if construct := orderingConstruct(n); construct != "" {
			uses = append(uses, schedulingUse{position: fset.Position(n.Pos()), construct: construct})
		}
		return true
	})
	return uses
}

// orderingConstruct names the goroutine, wait group, channel operation or
// timer n declares, or "" when n is none of them.
func orderingConstruct(n ast.Node) string {
	switch n := n.(type) {
	case *ast.GoStmt:
		return goStatement
	case *ast.ChanType:
		return channelType
	case *ast.SelectStmt:
		return selectStmt
	case *ast.SendStmt:
		return sendStmt
	case *ast.UnaryExpr:
		if n.Op == token.ARROW {
			return receiveExpr
		}
	case *ast.SelectorExpr:
		return selectorConstruct(n)
	}
	return ""
}

// selectorConstruct names the scheduling construct a selector expression is:
// sync.WaitGroup, or any AfterFunc, whether time's or a Scheduler facade's.
func selectorConstruct(n *ast.SelectorExpr) string {
	if pkg, ok := n.X.(*ast.Ident); ok && pkg.Name == "sync" && n.Sel.Name == "WaitGroup" {
		return waitGroup
	}
	if n.Sel.Name == "AfterFunc" {
		return afterFuncCall
	}
	return ""
}
