package engine

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Every terminal state the runtime writes outside Finish settles the Task
// at that moment. Finish stamps only the Tasks its own sweeps resolved, so a
// missed settle would move SettledAt to Finish and stretch Running and
// Total across the caller's later work.

const (
	settleAt       = time.Second
	laterCallerGap = 10 * time.Second
)

func newSettleOutput(t *testing.T, opts ...Option) (*Output, *manualClock) {
	t.Helper()
	clock := &manualClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	o := newOutput("", append([]Option{to(io.Discard), withNoColor(), withClock(clock)}, opts...)...)
	t.Cleanup(func() { _ = o.Close() })
	return o, clock
}

// taskInState returns the one Task in state, failing the test otherwise.
func taskInState(t *testing.T, o *Output, state EntityState) *taskState {
	t.Helper()
	o.mu.Lock()
	defer o.mu.Unlock()
	var found *taskState
	for _, st := range o.tasks {
		if st.state != state {
			continue
		}
		if found != nil {
			t.Fatalf("two Tasks in state %s: %q and %q", state, found.name, st.name)
		}
		found = st
	}
	if found == nil {
		t.Fatalf("no Task in state %s", state)
	}
	return found
}

func assertSettledAt(t *testing.T, st *taskState, want time.Time) {
	t.Helper()
	if got := st.timing.SettledAt; !got.Equal(want) {
		t.Fatalf("%q SettledAt = %v, want %v (settled at its terminal write, not at Finish)", st.name, got, want)
	}
}

func TestSettle_DuplicateSiblingFailureSettlesWhenDeclared(t *testing.T) {
	o, clock := newSettleOutput(t)
	start := clock.Now()
	g := o.Group("worktrees")
	g.Task("wt1").Skipped(Reason("clean"))
	clock.Advance(settleAt)
	g.Task("wt1")
	clock.Advance(laterCallerGap)
	_ = o.Finish()
	assertSettledAt(t, taskInState(t, o, Failed), start.Add(settleAt))
}

func TestSettle_CancelledPendingConfirmSettlesAtCancel(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close(); _ = w.Close() })
	o, clock := newSettleOutput(t, stdin(r))
	start := clock.Now()
	answered := make(chan struct{})
	go func() {
		defer close(answered)
		o.Confirm("delete origin/production-hotfix?")
	}()
	waitForPendingConfirm(t, o)
	clock.Advance(settleAt)
	o.cancelActive("interrupted")
	<-answered
	clock.Advance(laterCallerGap)
	_ = o.Finish()
	assertSettledAt(t, taskInState(t, o, Cancelled), start.Add(settleAt))
}

// pendingConfirmDeadline bounds the real-time wait for Confirm to register
// its abort channel; the domain clock is fake, so this only guards a hang.
const pendingConfirmDeadline = 5 * time.Second

func waitForPendingConfirm(t *testing.T, o *Output) {
	t.Helper()
	deadline := time.Now().Add(pendingConfirmDeadline)
	for time.Now().Before(deadline) {
		o.mu.Lock()
		pending := len(o.confirmAbort)
		o.mu.Unlock()
		if pending > 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("Confirm never registered a pending gate")
}

// stateWriters are the only functions that may assign a Task's state:
// settleAsLocked for every terminal write outside Finish, promoteRunningLocked
// for the one non-terminal move, and Finish's own sweeps, which
// settleUnstampedLocked stamps. A new direct write elsewhere would skip the
// settle stamp and the release of the Tasks waiting on it.
var stateWriters = map[string]bool{
	"settleAsLocked":             true,
	"promoteRunningLocked":       true,
	"Finish":                     true,
	"autoResolveGroupsLocked":    true,
	"resolveUnstartedTaskLocked": true,
}

func TestSettle_OnlySettleAsWritesTerminalState(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || stateWriters[fn.Name.Name] {
				continue
			}
			for _, pos := range stateAssignments(fn.Body) {
				t.Errorf("%s: %s assigns a Task state directly; call settleAsLocked", fset.Position(pos), fn.Name.Name)
			}
		}
	}
}

// stateAssignments returns where body assigns to a `.state` field.
func stateAssignments(body *ast.BlockStmt) []token.Pos {
	var found []token.Pos
	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, lhs := range assign.Lhs {
			if sel, ok := lhs.(*ast.SelectorExpr); ok && sel.Sel.Name == "state" {
				found = append(found, assign.Pos())
			}
		}
		return true
	})
	return found
}
