package engine

import (
	"errors"
	"go/ast"
	"go/build"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
)

// The constructs by which engine code could decide when work runs: start a
// goroutine, wait on a group of them, order by channel, or poll and wait on a
// timer.
const (
	goStatement   = "go statement"
	channelType   = "channel type"
	channelRange  = "range over a channel"
	channelClose  = "channel close"
	waitGroup     = "sync.WaitGroup"
	syncCond      = "sync.Cond"
	errgroupGroup = "errgroup.Group"
	selectStmt    = "select statement"
	sendStmt      = "channel send"
	receiveExpr   = "channel receive"
	afterFuncCall = "AfterFunc call"
	timerCall     = "time wait or timer"
)

// errgroupImportPath is the package whose Group starts and waits on
// goroutines; the engine does not import it, and may not start.
const errgroupImportPath = "golang.org/x/sync/errgroup"

// timerFunctions are the time package functions that wait, poll or schedule.
var timerFunctions = []string{"Sleep", "NewTimer", "NewTicker", "After", "Tick"}

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
// goroutine, waits on a sync.WaitGroup, sync.Cond or errgroup.Group, sleeps,
// or orders by channel (declare, send, receive, range, close, select) or timer
// except the exemptions above. The graph owns the scheduler wholly; a worker
// pool, a drain or a wake-up signal growing back here would be a second
// scheduler. The guard reads the package's types, so a shape cannot slip past
// by aliasing an import, hiding the channel behind a field or a result, or
// calling the method through a struct.
func TestEngineHoldsNoSchedulingDecision(t *testing.T) {
	pkg, err := build.ImportDir(".", 0)
	if err != nil {
		t.Fatalf("list engine sources: %v", err)
	}
	sources := make(map[string][]byte, len(pkg.GoFiles))
	for _, name := range pkg.GoFiles {
		src, readErr := os.ReadFile(name)
		if readErr != nil {
			t.Fatalf("read %s: %v", name, readErr)
		}
		sources[name] = src
	}
	uses := schedulingConstructsIn(t, sources)
	if len(uses) == 0 {
		t.Fatal("the guard found no scheduling construct, not even in the exempt files: it is not reading the package")
	}
	for _, use := range uses {
		exemption, exempt := concurrencyExemptions[use.file]
		if exempt && exemption.allows(use.construct) {
			continue
		}
		t.Errorf("%s: %s; scheduling belongs to internal/graph (exemptions: concurrencyExemptions)",
			use.position, use.construct)
	}
}

// guardFixtures are the guard's own proof: each construct it names, written so
// that it type-checks, including the shapes a syntax-only check missed and the
// ones that look harmless in isolation (a bare receive on ctx.Done() is a
// wait, and a select is an ordering decision).
var guardFixtures = map[string]struct {
	src  string
	want string
}{
	"go statement":                           {"func f() { go f() }", goStatement},
	"channel type":                           {"var c chan int", channelType},
	"receive-only channel field":             {"type s struct{ c <-chan int }", channelType},
	"select":                                 {"func f(c chan int) { select { default: } }", selectStmt},
	"empty select":                           {"func f() { select {} }", selectStmt},
	"send":                                   {"func f(c chan int) { c <- 1 }", sendStmt},
	"receive":                                {"import \"context\"\nfunc f(ctx context.Context) { <-ctx.Done() }", receiveExpr},
	"range over a channel the graph returns": {"func f(g interface{ Barrier() <-chan struct{} }) { for range g.Barrier() {} }", channelRange},
	"close a channel declared elsewhere":     {"type o struct{ closing chan struct{} }\nfunc f(x *o) { close(x.closing) }", channelClose},
	"wait group":                             {"import \"sync\"\nvar w sync.WaitGroup", waitGroup},
	"aliased sync":                           {"import s \"sync\"\nvar w s.WaitGroup", waitGroup},
	"WaitGroup.Go on a field":                {"import \"sync\"\ntype o struct{ wg sync.WaitGroup }\nfunc f(x *o) { x.wg.Go(func() {}) }", waitGroup},
	"WaitGroup.Wait on a field":              {"import \"sync\"\ntype o struct{ wg sync.WaitGroup }\nfunc f(x *o) { x.wg.Wait() }", waitGroup},
	"WaitGroup.Wait through an embedded one": {"import \"sync\"\ntype o struct{ sync.WaitGroup }\nfunc f(x *o) { x.Wait() }", waitGroup},
	"sync.Cond wait":                         {"import \"sync\"\nfunc f(c *sync.Cond) { c.Wait() }", syncCond},
	"errgroup.Go":                            {"import \"" + errgroupImportPath + "\"\nfunc f(g *errgroup.Group) { g.Go(func() error { return nil }) }", errgroupGroup},
	"time.AfterFunc":                         {"import \"time\"\nfunc f() { time.AfterFunc(0, f) }", afterFuncCall},
	"facade AfterFunc":                       {"func f(s interface{ AfterFunc() }) { s.AfterFunc() }", afterFuncCall},
	"time.Sleep poll":                        {"import \"time\"\nfunc f() { for !ready() { time.Sleep(1) } }\nfunc ready() bool { return true }", timerCall},
	"time.NewTimer":                          {"import \"time\"\nfunc f() { _ = time.NewTimer(1) }", timerCall},
	"time.NewTicker":                         {"import \"time\"\nfunc f() { _ = time.NewTicker(1) }", timerCall},
	"time.After":                             {"import \"time\"\nfunc f() { _ = time.After(1) }", timerCall},
	"time.Tick":                              {"import \"time\"\nfunc f() { _ = time.Tick(1) }", timerCall},
}

func TestSlice36_EngineGuardNamesEverySchedulingShape(t *testing.T) {
	for name, fixture := range guardFixtures {
		t.Run(name, func(t *testing.T) {
			found := false
			for _, use := range schedulingConstructsIn(t, map[string][]byte{"probe.go": fixtureSource(fixture.src)}) {
				found = found || use.construct == fixture.want
			}
			if !found {
				t.Errorf("guard missed %q in %q", fixture.want, fixture.src)
			}
		})
	}
}

// Code that tells time or holds a lock decides nothing about when work runs.
func TestEngineGuardLeavesHarmlessCodeAlone(t *testing.T) {
	harmless := map[string]string{
		"reading the clock":     "import \"time\"\nfunc f() time.Time { return time.Now().Add(time.Second) }",
		"a duration constant":   "import \"time\"\nconst d = 5 * time.Second",
		"a mutex":               "import \"sync\"\nvar mu sync.Mutex\nfunc f() { mu.Lock(); mu.Unlock() }",
		"a function that Waits": "type w struct{}\nfunc (w) Wait() {}\nfunc f(x w) { x.Wait() }",
		"a map and a slice":     "var m = map[string][]int{}",
	}
	for name, src := range harmless {
		t.Run(name, func(t *testing.T) {
			if uses := schedulingConstructsIn(t, map[string][]byte{"probe.go": fixtureSource(src)}); len(uses) > 0 {
				t.Errorf("guard flagged harmless code %q: %v", src, uses)
			}
		})
	}
}

// run_signal.go's exemption admits channel types only.
func TestRunSignalExemptionIsChannelTypesOnly(t *testing.T) {
	e := concurrencyExemptions["run_signal.go"]
	for _, c := range []string{goStatement, waitGroup, selectStmt, sendStmt, receiveExpr, channelClose, channelRange, afterFuncCall, timerCall} {
		if e.allows(c) {
			t.Errorf("run_signal.go exemption allows %s", c)
		}
	}
	if !e.allows(channelType) {
		t.Errorf("run_signal.go exemption refuses channel types")
	}
}

// fixtureSource is body as a source file of package engine.
func fixtureSource(body string) []byte { return []byte("package engine\n" + body) }

// schedulingUse is one scheduling construct found in a source file.
type schedulingUse struct {
	position  token.Position
	file      string
	construct string
}

// guardTypes is the source importer shared by every check, so the standard
// library and the engine's imports are type-checked once. It serves a stand-in
// for errgroup, which this module does not import.
var guardTypes = sync.OnceValues(func() (*guardImporter, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	source, ok := importer.ForCompiler(fset, "source", nil).(types.ImporterFrom)
	if !ok {
		return nil, errors.New("source importer does not resolve module imports")
	}
	return &guardImporter{fset: fset, source: source, dir: dir, errgroup: errgroupStandIn()}, nil
})

type guardImporter struct {
	fset     *token.FileSet
	source   types.ImporterFrom
	dir      string
	errgroup *types.Package
}

func (g *guardImporter) Import(path string) (*types.Package, error) {
	return g.ImportFrom(path, g.dir, 0)
}

func (g *guardImporter) ImportFrom(path, dir string, mode types.ImportMode) (*types.Package, error) {
	if path == errgroupImportPath {
		return g.errgroup, nil
	}
	return g.source.ImportFrom(path, dir, mode)
}

// errgroupStandIn is a package shaped like golang.org/x/sync/errgroup: a Group
// with a Go method.
func errgroupStandIn() *types.Package {
	pkg := types.NewPackage(errgroupImportPath, "errgroup")
	name := types.NewTypeName(token.NoPos, pkg, "Group", nil)
	group := types.NewNamed(name, types.NewStruct(nil, nil), nil)
	recv := types.NewVar(token.NoPos, pkg, "g", types.NewPointer(group))
	errorResult := types.NewTuple(types.NewVar(token.NoPos, nil, "", types.Universe.Lookup("error").Type()))
	work := types.NewSignatureType(nil, nil, nil, nil, errorResult, false)
	signature := types.NewSignatureType(recv, nil, nil, types.NewTuple(types.NewVar(token.NoPos, pkg, "f", work)), nil, false)
	group.AddMethod(types.NewFunc(token.NoPos, pkg, "Go", signature))
	pkg.Scope().Insert(name)
	pkg.MarkComplete()
	return pkg
}

// schedulingConstructsIn type-checks sources as one package of the engine and
// reports every scheduling construct in them.
func schedulingConstructsIn(t *testing.T, sources map[string][]byte) []schedulingUse {
	t.Helper()
	guard, err := guardTypes()
	if err != nil {
		t.Fatalf("load importer: %v", err)
	}
	names := make([]string, 0, len(sources))
	for name := range sources {
		names = append(names, name)
	}
	slices.Sort(names)
	files := make([]*ast.File, 0, len(names))
	for _, name := range names {
		file, parseErr := parser.ParseFile(guard.fset, filepath.Join(guard.dir, name), sources[name], 0)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", name, parseErr)
		}
		files = append(files, file)
	}
	info := &types.Info{
		Types:      map[ast.Expr]types.TypeAndValue{},
		Uses:       map[*ast.Ident]types.Object{},
		Selections: map[*ast.SelectorExpr]*types.Selection{},
	}
	config := types.Config{Importer: guard, Error: func(checkErr error) { t.Errorf("type-check: %v", checkErr) }}
	_, _ = config.Check(modulePathOfEngine, guard.fset, files, info)
	var uses []schedulingUse
	for i, file := range files {
		classify := schedulingClassifier{info: info}
		ast.Inspect(file, func(n ast.Node) bool {
			if construct := classify.construct(n); construct != "" {
				uses = append(uses, schedulingUse{position: guard.fset.Position(n.Pos()), file: names[i], construct: construct})
			}
			return true
		})
	}
	return uses
}

// modulePathOfEngine is the import path the checked engine sources compile as.
const modulePathOfEngine = "github.com/zachbornheimer/evident-output/internal/engine"

// schedulingClassifier names the scheduling construct a node is, from the
// types the checker resolved.
type schedulingClassifier struct{ info *types.Info }

// construct names the goroutine, wait, channel operation or timer n declares,
// or "" when n is none of them.
func (c schedulingClassifier) construct(n ast.Node) string {
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
	case *ast.RangeStmt:
		if c.isChannel(n.X) {
			return channelRange
		}
	case *ast.CallExpr:
		if c.isClose(n) {
			return channelClose
		}
	case *ast.SelectorExpr:
		return c.selectorConstruct(n)
	}
	return ""
}

func (c schedulingClassifier) isChannel(e ast.Expr) bool {
	typ := c.info.TypeOf(e)
	if typ == nil {
		return false
	}
	_, isChan := typ.Underlying().(*types.Chan)
	return isChan
}

// isClose reports whether call is the builtin close.
func (c schedulingClassifier) isClose(call *ast.CallExpr) bool {
	ident, ok := call.Fun.(*ast.Ident)
	if !ok {
		return false
	}
	builtin, ok := c.info.Uses[ident].(*types.Builtin)
	return ok && builtin.Name() == "close"
}

// selectorConstruct names what n selects when it is a scheduling type, one of
// its methods, a waiting time function, or any AfterFunc, whether time's or a
// Scheduler facade's.
func (c schedulingClassifier) selectorConstruct(n *ast.SelectorExpr) string {
	if n.Sel.Name == "AfterFunc" {
		return afterFuncCall
	}
	switch obj := c.info.Uses[n.Sel].(type) {
	case *types.TypeName:
		return schedulingTypeConstruct(obj.Pkg(), obj.Name())
	case *types.Func:
		if obj.Pkg() != nil && obj.Pkg().Path() == "time" && slices.Contains(timerFunctions, obj.Name()) {
			return timerCall
		}
		return methodConstruct(obj)
	}
	return ""
}

// methodConstruct names the scheduling type fn is a method of, or "".
func methodConstruct(fn *types.Func) string {
	signature, ok := fn.Type().(*types.Signature)
	if !ok || signature.Recv() == nil {
		return ""
	}
	recv := signature.Recv().Type()
	if pointer, isPointer := recv.(*types.Pointer); isPointer {
		recv = pointer.Elem()
	}
	named, ok := recv.(*types.Named)
	if !ok {
		return ""
	}
	return schedulingTypeConstruct(named.Obj().Pkg(), named.Obj().Name())
}

// schedulingTypeConstruct names the scheduling construct the type name in pkg
// is, or "".
func schedulingTypeConstruct(pkg *types.Package, name string) string {
	if pkg == nil {
		return ""
	}
	switch {
	case pkg.Path() == "sync" && name == "WaitGroup":
		return waitGroup
	case pkg.Path() == "sync" && name == "Cond":
		return syncCond
	case pkg.Path() == errgroupImportPath && name == "Group":
		return errgroupGroup
	}
	return ""
}
