package review

import (
	"go/ast"
	"slices"
	"strings"
)

// The 1.x entry points, all removed in 2.0: how a 1.x program started (Init,
// New, package-level Task/Group/Sequence, the one-argument Main over
// func(ctx) error, Print*, Config.Title, Output.Task). 2.0 has one entry,
// evo.Main(cfg, run) with func run(ctx context.Context, r *evo.Run) error,
// and every Task, Group and Sequence is declared on that r. Each rewrite
// below is a one-line "replace OLD with NEW" or "delete OLD; ..." edit, or a
// documented manual step (move statements into run, add an import).

// packageHandles are the 1.x package-level constructors whose 2.0 spelling
// is a method of *evo.Run; Output had the same three.
var packageHandles = map[string]bool{"Task": true, "Group": true, "Sequence": true}

// printFuncs maps a Print* removed in 2.0 to the fmt function that writes the
// same text to Run.Text().
var printFuncs = map[string]string{"Print": "Fprint", "Printf": "Fprintf", "Println": "Fprintln"}

// defaultRunVar is the name this rewrite gives the *evo.Run run receives.
const defaultRunVar = "r"

// entryScan is what one pass over a 1.x file learns before any rewrite is
// worded: the Config every entry point folds into, the statements that
// declare an Output, and the funcs handed to Main as the run callback.
type entryScan struct {
	hostConfig string
	// hostStmts are the statements that bind an Output from Init or New
	// (both removed in 2.0); the rewrite deletes the whole statement.
	hostStmts map[ast.Expr]ast.Stmt
	runDecls  map[string]*ast.FuncDecl
	// runLits are the func literals handed to a removed Main, Run or
	// Output.Run (removed in 2.0) as the run callback, still with the 1.x
	// one-parameter signature.
	runLits []*ast.FuncLit
	needs   map[string]bool
	// executes is true when the file starts its run with evo.Run or
	// Output.Run (both removed in 2.0), whose 2.0 spelling is
	// evo.Execute(ctx, cfg, run).
	executes bool
}

func (d *removedTwoZeroDetector) scanEntryPoints() {
	d.entry = entryScan{hostConfig: d.pkg + ".Config{}", hostStmts: map[ast.Expr]ast.Stmt{}, runDecls: map[string]*ast.FuncDecl{}, needs: map[string]bool{}}
	var configFound bool
	ast.Inspect(d.file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			if len(n.Rhs) == 1 {
				if isOutputConstructor(n.Rhs[0], d.pkg) {
					d.entry.hostStmts[n.Rhs[0]] = n
				}
			}
		case *ast.CallExpr:
			if call, ok := d.isEvoCall(n, "Init"); ok && len(call.Args) == 1 && !configFound {
				if cl, ok := call.Args[0].(*ast.CompositeLit); ok {
					d.entry.hostConfig, configFound = d.configFromLiteral(cl), true
				}
			}
			if call, ok := d.isEvoCall(n, "New"); ok && !configFound {
				d.entry.hostConfig, configFound = d.configFromOptions(call.Args), true
			}
			if call, ok := d.isEvoCall(n, "Main"); ok && len(call.Args) == 1 {
				d.noteRunCallback(call.Args[0])
			}
			if d.isExecutingRun(n) {
				d.entry.executes = true
				d.noteRunCallback(n.Args[1])
			}
		}
		return true
	})
}

func (d *removedTwoZeroDetector) need(pkg string) { d.entry.needs[pkg] = true }

// isExecutingRun reports whether call is the 1.x two-argument Run that
// returned a Result: the package-level Run or Output.Run (both removed in 2.0).
func (d *removedTwoZeroDetector) isExecutingRun(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Run" || len(call.Args) != 2 {
		return false
	}
	return isEvoIdent(sel.X, d.pkg) || d.receiverOf(sel.X) == outputReceiver
}

// noteRunCallback records the package func or func literal passed as a run
// callback, when it still has the 1.x one-parameter signature.
func (d *removedTwoZeroDetector) noteRunCallback(arg ast.Expr) {
	if lit, ok := arg.(*ast.FuncLit); ok {
		if lit.Type.Params.NumFields() == 1 {
			d.entry.runLits = append(d.entry.runLits, lit)
		}
		return
	}
	name := identName(arg)
	for _, decl := range d.file.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok && fd.Recv == nil && fd.Name.Name == name && fd.Type.Params.NumFields() == 1 {
			d.entry.runDecls[name] = fd
		}
	}
}

// configFromLiteral is a 1.x Config literal as 2.0 spells it: Title becomes
// Name and Options become the fields they stand for.
func (d *removedTwoZeroDetector) configFromLiteral(cl *ast.CompositeLit) string {
	set := map[string]bool{}
	for _, elt := range cl.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			set[identName(kv.Key)] = true
		}
	}
	var fields []string
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		switch {
		case !ok:
			fields = append(fields, d.nodeSrc(elt))
		case identName(kv.Key) == "Title":
			fields = append(fields, "Name: "+d.nodeSrc(kv.Value))
		case identName(kv.Key) == "Options":
			if slice, ok := kv.Value.(*ast.CompositeLit); ok {
				if mapped, ok := d.optionSliceFields(slice, set); ok {
					fields = append(fields, mapped)
				}
			}
		default:
			fields = append(fields, d.nodeSrc(elt))
		}
	}
	return d.pkg + ".Config{" + strings.Join(fields, ", ") + "}"
}

// configFromOptions is the Config the one-to-one Options stand for; an
// Option with no Config field has a finding of its own.
func (d *removedTwoZeroDetector) configFromOptions(options []ast.Expr) string {
	var fields []string
	for _, o := range options {
		if call, ok := o.(*ast.CallExpr); ok {
			if field, ok := d.optionField(call); ok {
				fields = append(fields, field)
			}
		}
	}
	return d.pkg + ".Config{" + strings.Join(fields, ", ") + "}"
}

// runVarAt is the name of the *evo.Run in scope at n: the enclosing func's
// own parameter, or the one this rewrite adds.
func (d *removedTwoZeroDetector) runVarAt(n ast.Node) string {
	off := d.offset(n)
	for _, lit := range d.entry.runLits {
		if d.nodeSpan(lit).contains(off) {
			return runVarForLit(lit)
		}
	}
	for _, decl := range d.file.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok && d.nodeSpan(fd).contains(off) {
			return d.runVarFor(fd)
		}
	}
	return defaultRunVar
}

func (d *removedTwoZeroDetector) runVarFor(fd *ast.FuncDecl) string {
	for _, field := range fd.Type.Params.List {
		if star, ok := field.Type.(*ast.StarExpr); ok && len(field.Names) == 1 {
			if sel, ok := star.X.(*ast.SelectorExpr); ok && sel.Sel.Name == "Run" && isEvoIdent(sel.X, d.pkg) {
				return field.Names[0].Name
			}
		}
	}
	if d.entry.runDecls[fd.Name.Name] != fd || !usesIdent(fd, defaultRunVar) {
		return defaultRunVar
	}
	return "evoRun"
}

// runVarForLit is the name this rewrite gives the *evo.Run a run literal
// receives.
func runVarForLit(lit *ast.FuncLit) string {
	if usesIdent(lit, defaultRunVar) {
		return "evoRun"
	}
	return defaultRunVar
}

func usesIdent(n ast.Node, name string) bool {
	var found bool
	ast.Inspect(n, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == name {
			found = true
		}
		return !found
	})
	return found
}

// recvSrc is a Task receiver as 2.0 spells it: a package-level or Output
// constructor at the root of the chain becomes a method of the run's r.
func (d *removedTwoZeroDetector) recvSrc(x ast.Expr) string {
	call, ok := x.(*ast.CallExpr)
	if !ok {
		return d.nodeSrc(x)
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return d.nodeSrc(x)
	}
	root := d.nodeSrc(sel.X)
	if packageHandles[sel.Sel.Name] && (isEvoIdent(sel.X, d.pkg) || d.receiverOf(sel.X) == outputReceiver) {
		root = d.runVarAt(call)
	} else if _, chained := sel.X.(*ast.CallExpr); chained {
		root = d.recvSrc(sel.X)
	}
	return root + "." + sel.Sel.Name + "(" + d.callArgsSrc(call.Args) + ")"
}

// inspectEntryCall migrates a removed package-level entry point.
func (d *removedTwoZeroDetector) inspectEntryCall(call *ast.CallExpr, name string) {
	switch {
	case name == "Init":
		stmt := d.entry.hostStmts[call]
		if stmt == nil {
			stmt = d.stmtOf(call)
		}
		d.reportHostConstruction(call, "evo.Init", d.nodeSrc(call), stmt, "its Config moves into "+d.entryCall(), false)
		d.cover(call)
	case name == "New":
		d.reportHostConstruction(call, "evo.New", d.nodeSrc(call), d.entry.hostStmts[call], "its Options move into the Config of evo.Main(cfg, run)", true)
	case packageHandles[name]:
		run := d.runVarAt(call)
		d.report(call, "evo."+name+" was removed in 2.0; a "+name+" is declared on the *evo.Run that run receives",
			"replace "+d.nodeSrc(call)+" with "+run+"."+name+"("+d.callArgsSrc(call.Args)+")")
	case name == "Main" && len(call.Args) == 1:
		d.report(call, "evo.Main(run) was removed in 2.0; Main takes the Config first",
			d.replaceRunCall(call, d.pkg+".Main("+d.entry.hostConfig+", ", call.Args[0]))
		d.cover(call)
	case printFuncs[name] != "":
		d.reportPrint(call, "evo."+name, d.runVarAt(call), name)
	}
}

// entryCall is the 2.0 call the program's Config moves into: Execute when it
// ran through the package-level Run, else Main.
func (d *removedTwoZeroDetector) entryCall() string {
	if d.entry.executes {
		return d.pkg + ".Execute(ctx, cfg, run)"
	}
	return d.pkg + ".Main(cfg, run)"
}

// reportHostConstruction words Init and New (both removed in 2.0) alike: the
// call is deleted when it is a statement of its own; New also moves the work
// that followed it into run.
func (d *removedTwoZeroDetector) reportHostConstruction(call *ast.CallExpr, label, old string, stmt ast.Stmt, moved string, moveToRun bool) {
	message := label + " was removed in 2.0; construction is " + d.pkg + ".Main(cfg, run) over func run(ctx context.Context, r *" + d.pkg + ".Run) error"
	suggestion := "delete " + old + "; " + moved
	if stmt != nil {
		suggestion = "delete " + d.nodeSrc(stmt) + "; " + moved
		if moveToRun {
			suggestion += ", then move the statements that followed it into func run(ctx context.Context, r *" + d.pkg + ".Run) error ending in return nil"
		}
	}
	d.report(call, message, suggestion)
}

// stmtOf is the expression statement that is exactly call, or nil.
func (d *removedTwoZeroDetector) stmtOf(call *ast.CallExpr) ast.Stmt {
	var found ast.Stmt
	ast.Inspect(d.file, func(n ast.Node) bool {
		if s, ok := n.(*ast.ExprStmt); ok && s.X == ast.Expr(call) {
			found = s
		}
		return found == nil
	})
	return found
}

// reportPrint rewrites a Print* call (removed in 2.0) to the
// fmt call that writes to Run.Text().
func (d *removedTwoZeroDetector) reportPrint(call *ast.CallExpr, label, run, name string) {
	args := d.callArgsSrc(call.Args)
	d.need("fmt")
	d.report(call, label+" was removed in 2.0; free human text is "+printFuncs[name]+" to "+run+".Text()",
		"replace "+d.nodeSrc(call)+" with fmt."+printFuncs[name]+"("+run+".Text(), "+args+")")
	d.cover(call)
}

// reportRunDecl adds the *evo.Run parameter a 2.0 run callback receives.
func (d *removedTwoZeroDetector) reportRunDecl(fd *ast.FuncDecl) {
	header := d.runHeader(fd, fd.Type, fd.Body, d.runVarFor(fd))
	d.report(fd, "func "+fd.Name.Name+"(ctx) error was removed in 2.0; a run callback receives the *"+d.pkg+".Run",
		"replace "+strings.TrimSpace(d.src[d.offset(fd):d.offset(fd.Body)])+" with "+strings.TrimSpace(header))
}

// runHeader is a 1.x run callback's header (up to its body) with the
// *evo.Run parameter a 2.0 run receives added after the context.
func (d *removedTwoZeroDetector) runHeader(from ast.Node, sig *ast.FuncType, body *ast.BlockStmt, runVar string) string {
	params := sig.Params
	field := params.List[0]
	text := d.src[d.offset(field):d.nodeSpan(field).end]
	if len(field.Names) == 0 {
		text = "ctx " + text
	}
	return d.src[d.offset(from):d.offset(params)+1] + text + ", " + runVar + " *" + d.pkg + ".Run" +
		d.src[d.nodeSpan(params).end-1:d.offset(body)]
}

// replaceRunCall words "replace OLD with NEW" for a call that hands cb on
// as the run callback after prefix. A 1.x func literal gains the *evo.Run
// parameter, and only the span up to its body is edited: the body stays
// where it is, because a one-line suggestion cannot carry its line breaks.
func (d *removedTwoZeroDetector) replaceRunCall(call *ast.CallExpr, prefix string, cb ast.Expr) string {
	lit, ok := cb.(*ast.FuncLit)
	if !ok || !slices.Contains(d.entry.runLits, lit) {
		return "replace " + d.nodeSrc(call) + " with " + prefix + d.nodeSrc(cb) + ")"
	}
	old := strings.TrimSpace(d.src[d.offset(call):d.offset(lit.Body)])
	return "replace " + old + " with " + prefix + strings.TrimSpace(d.runHeader(lit, lit.Type, lit.Body, runVarForLit(lit)))
}

// reportExitFinish migrates the 1.x way to end a hosted Output (Finish,
// removed in 2.0) to the exit code evo.Main returns.
func (d *removedTwoZeroDetector) reportExitFinish(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || identName(sel.X) != "os" || sel.Sel.Name != "Exit" || len(call.Args) != 1 {
		return false
	}
	finish, ok := call.Args[0].(*ast.CallExpr)
	if !ok {
		return false
	}
	fsel, ok := finish.Fun.(*ast.SelectorExpr)
	if !ok || fsel.Sel.Name != "Finish" || d.receiverOf(fsel.X) != outputReceiver {
		return false
	}
	d.report(call, "Output.Finish was removed in 2.0; Main finishes the run and returns the exit code",
		"replace "+d.nodeSrc(call)+" with os.Exit("+d.pkg+".Main("+d.entry.hostConfig+", run))")
	d.cover(call)
	return true
}

// reportMissingImports names the packages the rewrites above use and the
// file does not import.
func (d *removedTwoZeroDetector) reportMissingImports() {
	var missing []string
	for _, pkg := range []string{"context", "fmt"} {
		if d.entry.needs[pkg] && !d.imports(pkg) {
			missing = append(missing, `"`+pkg+`"`)
		}
	}
	if len(missing) == 0 || len(d.file.Imports) == 0 {
		return
	}
	d.report(d.file.Imports[0], "the 2.0 rewrites in this file use packages it does not import",
		"add the imports "+strings.Join(missing, " and "))
}

func (d *removedTwoZeroDetector) imports(path string) bool {
	for _, imp := range d.file.Imports {
		if strings.Trim(imp.Path.Value, `"`) == path {
			return true
		}
	}
	return false
}
