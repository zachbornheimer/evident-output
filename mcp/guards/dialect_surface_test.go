package guards_test

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/apisurface"
)

// dialectSurface is the CONTRACT.md public function set. Keys are receivers
// ("pkg", "*Run", "*Task", ...). Values are `Name(args)` strings, the exact
// exported surface. A new export, a dropped contract verb, or a signature
// change fails this test.
var dialectSurface = map[string][]string{
	"pkg": {
		"CleanStop(signals ...os.Signal)",
		"Code(value string)",
		"Compute(task *Task, fn func(context.Context) (T, error))",
		"Destructive()",
		"Detail(text string)",
		"Effect(ctx context.Context, spec EffectSpec, fn func(context.Context) error)",
		"Exclude(reason string, opts ...ProblemOption)",
		"Exec(ctx context.Context, spec ExecSpec)",
		"Execute(ctx context.Context, cfg Config, run RunFunc)",
		"Fail(summary string, opts ...ProblemOption)",
		"File(ctx context.Context, spec FileSpec)",
		"Main(cfg Config, run RunFunc)",
		"Next(action Action)",
		"NonInteractive(argv ...string)",
		"On(subject string)",
		"PartialEffect(committed int, err error)",
		"Patch(ctx context.Context, diff []byte)",
		"Refuse(summary string, opts ...ProblemOption)",
	},
	"*Run": {
		"Confirm(question string, opts ...ConfirmOption)",
		"Fact(name string, value string)",
		"Group(name string)",
		"ResultWriter()",
		"Sequence(name string)",
		"SlogHandler()",
		"Summary(text string)",
		"Suspend(fn func() error)",
		"Task(name string)",
		"Text()",
	},
	"*Task": {
		"After(preds ...Predecessor)",
		"Define(fn func(context.Context) error, opts ...DefineOption)",
		"Doing(text string, args ...any)",
		"Fact(name string, value string)",
		"Problem(summary string, opts ...ProblemOption)",
		"Progress(completed int, total int)",
		"Summary(text string)",
		"Verify(fn func(context.Context) (bool, error))",
		"Writer()",
	},
	"*Group": {
		"After(preds ...Predecessor)",
		"Build(fn func(*Group))",
		"Group(name string)",
		"Sequence(name string)",
		"Summary(text string)",
		"Task(name string)",
	},
	"*Sequence": {
		"After(preds ...Predecessor)",
		"Build(fn func(*Sequence))",
		"Group(name string)",
		"Sequence(name string)",
		"Summary(text string)",
		"Task(name string)",
	},
	"Result": {
		"ExitCode()",
	},
}

func TestDialectSurface_PublicFuncsAreExactlyTheRecSet(t *testing.T) {
	got := exportedFuncsByRecv(t)
	recvs := make([]string, 0, len(dialectSurface)+len(got))
	seen := map[string]struct{}{}
	for recv := range dialectSurface {
		recvs = append(recvs, recv)
		seen[recv] = struct{}{}
	}
	for recv := range got {
		if _, ok := seen[recv]; ok {
			continue
		}
		recvs = append(recvs, recv)
	}
	sort.Strings(recvs)

	var failed bool
	for _, recv := range recvs {
		want := append([]string(nil), dialectSurface[recv]...)
		have := append([]string(nil), got[recv]...)
		sort.Strings(want)
		sort.Strings(have)
		if reflect.DeepEqual(want, have) {
			continue
		}
		failed = true
		extra, missing := diffSorted(have, want)
		t.Errorf("%s: exported funcs != rec set\nEXTRA (%d):\n  %s\nMISSING (%d):\n  %s",
			recv, len(extra), joinOrNone(extra), len(missing), joinOrNone(missing))
	}
	if failed {
		t.Errorf("public func surface != evo-rec.md (+ *f). Expansion of the exported set is a fail.")
	}
}

func TestDialectSurface_NoNewAPIConstructor(t *testing.T) {
	got := exportedFuncsByRecv(t)
	for _, sig := range got["pkg"] {
		if strings.HasPrefix(sig, "NewAPI(") {
			t.Errorf("pkg extra %s: API belongs on Init/Config, not a second constructor", sig)
		}
	}
	if _, ok := got["*API"]; ok {
		t.Error("*API methods are extra: fold into Run")
	}
}

func TestDialectSurface_TaskDeclareIsNameOnly(t *testing.T) {
	got := exportedFuncsByRecv(t)
	if !containsSig(got["*Run"], "Task(name string)") {
		t.Errorf("*Run.Task want Task(name string); got %v", got["*Run"])
	}
	if sig := findSig(got["pkg"], "Task("); sig != "(absent)" {
		t.Errorf("evo.Task was removed in 2.0 (declare through *Run.Task) but is exported: %s", sig)
	}
}

// TestDialectSurface_LegacyMutationVerbsAreRemoved pins ZYS-950: the seven
// Task mutation verbs were removed in 1.1 with no aliases. Opaque
// mutations go through evo.Effect; file state goes through evo.File.
func TestDialectSurface_LegacyMutationVerbsAreRemoved(t *testing.T) {
	got := exportedFuncsByRecv(t)
	for _, verb := range []string{"Add(", "Create(", "Delete(", "Push(", "Remove(", "Update(", "Write("} {
		if sig := findSig(got["*Task"], verb); sig != "(absent)" {
			t.Errorf("*Task.%s was removed in 1.1 but is exported: %s", strings.TrimSuffix(verb, "("), sig)
		}
	}
	if sig := findSig(got["pkg"], "Affected("); sig != "(absent)" {
		t.Errorf("evo.Affected was removed in 1.1 (EffectSpec.Quantity replaces it) but is exported: %s", sig)
	}
}

func TestDialectSurface_SuspendIsExported(t *testing.T) {
	got := exportedFuncsByRecv(t)
	if !containsSig(got["*Run"], "Suspend(fn func() error)") {
		t.Errorf("*Run.Suspend want Suspend(fn func() error); got matching %s",
			findSig(got["*Run"], "Suspend("))
	}
}

func exportedFuncsByRecv(t *testing.T) map[string][]string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string][]string)
	seen := make(map[string]map[string]struct{})
	collectExportedFuncs(t, wd, "evo", true, out, seen)
	for recv := range out {
		sort.Strings(out[recv])
	}
	return out
}

func collectExportedFuncs(t *testing.T, dir, pkg string, includePkg bool, out map[string][]string, seen map[string]map[string]struct{}) {
	t.Helper()
	fset := token.NewFileSet()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir %s: %v", dir, err)
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		if f.Name.Name != pkg {
			continue
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || !fn.Name.IsExported() {
				continue
			}
			recv := "pkg"
			if fn.Recv != nil && len(fn.Recv.List) > 0 {
				recv = typeName(fn.Recv.List[0].Type)
				if recv == "" || !exportedType(recv) {
					continue
				}
			} else if !includePkg {
				continue
			}
			sig := fn.Name.Name + paramList(fset, fn.Type.Params)
			if seen[recv] == nil {
				seen[recv] = map[string]struct{}{}
			}
			if _, ok := seen[recv][sig]; ok {
				continue
			}
			seen[recv][sig] = struct{}{}
			out[recv] = append(out[recv], sig)
		}
	}
}

func paramList(fset *token.FileSet, fl *ast.FieldList) string {
	if fl == nil || len(fl.List) == 0 {
		return "()"
	}
	var parts []string
	for _, field := range fl.List {
		typ := exprString(fset, field.Type)
		if len(field.Names) == 0 {
			parts = append(parts, typ)
			continue
		}
		for _, n := range field.Names {
			parts = append(parts, n.Name+" "+typ)
		}
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

func exprString(fset *token.FileSet, expr ast.Expr) string {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, expr); err != nil {
		return fmt.Sprintf("%T", expr)
	}
	return buf.String()
}

func typeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return "*" + id.Name
		}
	}
	return ""
}

func exportedType(recv string) bool {
	name := strings.TrimPrefix(recv, "*")
	return len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z'
}

func diffSorted(have, want []string) (extra, missing []string) {
	i, j := 0, 0
	for i < len(have) && j < len(want) {
		switch {
		case have[i] == want[j]:
			i++
			j++
		case have[i] < want[j]:
			extra = append(extra, have[i])
			i++
		default:
			missing = append(missing, want[j])
			j++
		}
	}
	extra = append(extra, have[i:]...)
	missing = append(missing, want[j:]...)
	return extra, missing
}

func joinOrNone(ss []string) string {
	if len(ss) == 0 {
		return "(none)"
	}
	return strings.Join(ss, "\n  ")
}

func containsSig(sigs []string, want string) bool {
	return slices.Contains(sigs, want)
}

func findSig(sigs []string, prefix string) string {
	var match []string
	for _, s := range sigs {
		if strings.HasPrefix(s, prefix) {
			match = append(match, s)
		}
	}
	if len(match) == 0 {
		return "(absent)"
	}
	return strings.Join(match, ", ")
}

func TestDialectSurface_SubsetOfGoDocSurface(t *testing.T) {
	live, err := apisurface.Walk(".")
	if err != nil {
		t.Fatal(err)
	}
	var missing []string
	for recv, sigs := range dialectSurface {
		for _, sig := range sigs {
			prefix := dialectGoDocPrefix(recv, sig)
			if !surfaceHasPrefix(live, prefix) {
				missing = append(missing, recv+" "+sig)
			}
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("dialect methods missing from go/doc surface:\n  %s", strings.Join(missing, "\n  "))
	}
}

func dialectGoDocPrefix(recv, sig string) string {
	name, _, _ := strings.Cut(sig, "(")
	if recv == "pkg" {
		return "func " + name + "("
	}
	return "func (" + strings.TrimPrefix(recv, "*") + ") " + name + "("
}

func surfaceHasPrefix(live []string, prefix string) bool {
	for _, line := range live {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}
