package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/rules"
)

// removedArm is one 1.x call shape and the 2.0 spelling API-032 must give
// for it: the suggestion lands on the 2.0 surface directly, never on a 1.x
// intermediate.
type removedArm struct {
	name, body, want string
}

var removedArms = []removedArm{
	{
		"Block", `func f(task *evo.TaskHandle) { task.Block("no remote", evo.NextCommand("git", "remote", "add")) }`,
		`return evo.Refuse("no remote", evo.Next(evo.Action{Command: []string{"git", "remote", "add"}}))`,
	},
	{
		"Blockf", `func f(task *evo.TaskHandle, n int) { task.Blockf("%d refs", n) }`,
		`return evo.Refuse(fmt.Sprintf("%d refs", n))`,
	},
	{
		"Fail with Cause", `func f(task *evo.TaskHandle, err error) { task.Fail("lint failed", evo.Cause(err)) }`,
		`return evo.Fail("lint failed", evo.Detail(err.Error()))`,
	},
	{
		"Failf wrapping an error", `func f(task *evo.TaskHandle, err error) { task.Failf("install: %w", err) }`,
		`return evo.Fail("install", evo.Detail(err.Error()))`,
	},
	{
		"Skipped", `func f(task *evo.TaskHandle) { task.Skipped(evo.Reason("protected")) }`,
		`return evo.Exclude("protected")`,
	},
	{
		"Kept", `func f(task *evo.TaskHandle, reason string) { task.Kept(reason) }`,
		`task.Verify(func(ctx context.Context) (bool, error) { return <already in place>, nil })`,
	},
	{
		"Warn drops Severity", `func f(task *evo.TaskHandle) { task.Warn("stale", evo.Severity(evo.SeverityWarning)) }`,
		`task.Problem("stale")`,
	},
	{
		"Step", `func f(task *evo.TaskHandle) { task.Step(1, 3, "file.go") }`,
		`task.Progress(1, 3).Doing("file.go")`,
	},
	{
		"Done with text", `func f(task *evo.TaskHandle) { task.Done("3 checked") }`,
		`task.Summary("3 checked").Define(func(ctx context.Context) error { return nil })`,
	},
	{
		"Done with format", `func f(task *evo.TaskHandle, n int) { task.Done("%d stale", n) }`,
		`task.Summary(fmt.Sprintf("%d stale", n)).Define(`,
	},
	{
		"Evidence", `func f(task *evo.TaskHandle) { _ = task.Evidence() }`,
		`task.Writer()`,
	},
	{
		"object-first Delete", `func f(task *evo.TaskHandle, n int) { task.Delete("local tip", func() error { return nil }, evo.Affected(n)) }`,
		`task.Define(func(ctx context.Context) error { return evo.Effect(ctx, evo.EffectSpec{Verb: evo.Delete, Object: "local tip", Quantity: n}, func(context.Context) error { return nil }) })`,
	},
	{
		"Push becomes Update", `func f(task *evo.TaskHandle) { task.Push("ref", nil, evo.Affected(2)) }`,
		`Verb: evo.Update, Object: "ref", Quantity: 2`,
	},
	{
		"Write", `func f(task *evo.TaskHandle, path string) { task.Write(path, func() error { return nil }) }`,
		`evo.File(ctx, evo.FileSpec{Path: path, Contents: data})`,
	},
	{
		"Record", `func f(task *evo.TaskHandle) { task.Record("install", 1, "package") }`,
		`evo.EffectSpec{Verb: evo.Create, Object: "package", Quantity: 1}`,
	},
	{
		"RecordLabel", `func f(task *evo.TaskHandle, n int) { task.RecordLabel("stale", n, "refs") }`,
		`task.Fact("stale", fmt.Sprintf("%d %s", n, "refs"))`,
	},
	{
		"RecordName write", `func f(task *evo.TaskHandle, p string) { task.RecordName("write", p) }`,
		`evo.File(ctx, evo.FileSpec{Path: p, Contents: data})`,
	},
	{"ExecSpec literal Args", `var _ = evo.ExecSpec{Executable: "go", Args: []string{"build", "./..."}}`, `with Argv: []string{"go", "build", "./..."}`},
	{"ExecSpec Args variable", `func f(exe string, args []string) { _ = evo.ExecSpec{Executable: exe, Args: args} }`, `with Argv: append([]string{exe}, args...)`},
	{"ExecSpec without Args", `var _ = evo.ExecSpec{Executable: "go"}`, `with Argv: []string{"go"}`},
	{"ExecSpec keeps the field between", `func f(exe, d string, args []string) { _ = evo.ExecSpec{Executable: exe, Dir: d, Args: args} }`, `with Argv: append([]string{exe}, args...), Dir: d`},
	{"Title option", `var _ = evo.Title("tool")`, `Name: "tool"`},
	{"To option", `func f(w io.Writer) { _ = evo.To(w) }`, `Facades: evo.Facades{Stdout: w}`},
	{"Plain option", `var _ = evo.Plain()`, `Output: evo.Plain`},
	{"Clock option", `func f(c evo.Clock) { _ = evo.Clock(c) }`, `Facades: evo.Facades{Clock: c}`},
	{"Terminal option", `func f(d evo.Terminal) { _ = evo.Terminal(d) }`, `Facades: evo.Facades{Terminal: d}`},
	{"Width option has no field", `var _ = evo.Width(80)`, `no 2.0 equivalent`},
	{"Config.Options", `var _ = evo.Config{Options: []evo.Option{evo.Title("t"), evo.DryRun()}}`, `Name: "t", DryRun: true`},
}

func TestAPI032_RemovedCallLandsOnTwoZero(t *testing.T) {
	for _, arm := range removedArms {
		t.Run(arm.name, func(t *testing.T) {
			res := review.GoSource("arm.go", evoIOBody(arm.body+"\n"))
			found := findAPI032(res)
			if !strings.Contains(joinSuggestions(found), arm.want) {
				t.Fatalf("want a suggestion containing %q, got %q\nfindings: %+v", arm.want, joinSuggestions(found), found)
			}
			for _, f := range found {
				if hits := rules.UnexplainedRetired(f.Suggestion + " " + f.Message); len(hits) > 0 {
					t.Errorf("%q teaches retired %s", f.Message, hits[0].Symbol.Contract)
				}
				if _, ok := rules.Explain(f.RuleID); !ok {
					t.Errorf("rules.Explain(%q) failed", f.RuleID)
				}
			}
		})
	}
}

// Done inside its own Task's Define callback is already defined: the call is
// deleted, or only its text stays as Summary.
func TestAPI032_DoneInsideOwnDefineDropsTheCall(t *testing.T) {
	src := evoBody(`func run(r *evo.Run) {
	scan := r.Task("scan")
	scan.Define(func(ctx context.Context) error {
		scan.Done("7 repos")
		return nil
	})
}
`)
	if got := joinSuggestions(findAPI032(review.GoSource("p.go", src))); !strings.Contains(got, `replace scan.Done("7 repos") with scan.Summary("7 repos")`) {
		t.Fatalf("suggestion = %q", got)
	}
}

// The Argv rewrite is on the first review of a 1.x program, before its import
// path is migrated, not only once the program imports 2.0.
func TestAPI032_ExecSpecArgvRewriteComesWithTheOneXImport(t *testing.T) {
	src := `package p

import evo "github.com/zachbornheimer/evident-output"

func f(exe string, args []string) { _ = evo.ExecSpec{Executable: exe, Args: args} }
`
	got := joinSuggestions(findAPI032(review.GoSource("p.go", src)))
	if want := `replace Executable: exe, Args: args with Argv: append([]string{exe}, args...)`; !strings.Contains(got, want) {
		t.Fatalf("want a suggestion containing %q, got %q", want, got)
	}
}

// A same-named method on another type, or a Task method on a receiver the
// file does not prove is a Task, is not a removed call.
func TestAPI032_LookalikesAreNotRemovedCalls(t *testing.T) {
	src := evoIOBody(`type recorder struct{}
func (recorder) Record(a, b, c string)   {}
func (recorder) Add(k, v string)         {}
func (recorder) Done()                   {}
func (recorder) Block(msg string)        {}
func f(h recorder, wg interface{ Done() }, rows interface{ Next() bool }, w io.Writer) {
	h.Record("a", "b", "c")
	h.Add("k", "v")
	h.Done()
	wg.Done()
	h.Block("x")
	for rows.Next() {
	}
	_ = w
}
`)
	if found := findAPI032(review.GoSource("p.go", src)); len(found) != 0 {
		t.Fatalf("lookalike methods flagged: %+v", found)
	}
}

// A remedy is an option of the ending it explains, so removed Next and
// NextCommand methods get guidance, never a blind replace.
func TestAPI032_RemovedRemedyMethodsAreGuidance(t *testing.T) {
	for name, body := range map[string]string{
		"task":    `func f(task *evo.TaskHandle) { task.NextCommand("git", "status") }`,
		"output":  `func f(out *evo.Output) { out.Next(evo.Label("retry")) }`,
		"failure": `func f(fail *evo.Failure) { fail.NextCommand("git", "status") }`,
	} {
		t.Run(name, func(t *testing.T) {
			found := guidanceFindings(findAPI032(review.GoSource("p.go", evoBody(body+"\n"))))
			if len(found) != 1 || strings.HasPrefix(found[0].Suggestion, "replace ") || !strings.Contains(found[0].Suggestion, "evo.Next(") {
				t.Fatalf("want one guidance finding naming evo.Next, got %+v", found)
			}
		})
	}
}

// An Option that has no one-to-one Config field, or whose field the literal
// already sets, gets a message and no rewrite that would drop or duplicate it.
func TestAPI032_OptionsThatCannotBeRewrittenOfferNoEdit(t *testing.T) {
	for name, cfg := range map[string]string{
		"no field":    `evo.Config{Options: []evo.Option{evo.Width(80), evo.Title("t")}}`,
		"already set": `evo.Config{Name: "t", Options: []evo.Option{evo.Title("u")}}`,
		"repeated":    `evo.Config{Options: []evo.Option{evo.Title("t"), evo.Title("u")}}`,
	} {
		t.Run(name, func(t *testing.T) {
			found := findAPI032(review.GoSource("p.go", evoBody("var _ = "+cfg+"\n")))
			if len(found) == 0 {
				t.Fatal("Config.Options not reported")
			}
			if sug := found[0].Suggestion; strings.HasPrefix(sug, "replace "+"evo.Config") || strings.Contains(sug, "Name: \"t\", Name") {
				t.Fatalf("suggestion rewrites into a lossy or duplicate Config: %q", sug)
			}
		})
	}
}

// Before 2.0 these calls are live API, so a pin below 2.0 reports none.
func TestAPI032_RemovedCallsAreSilentBelowTwoZero(t *testing.T) {
	src := evoBody(`func f(task *evo.TaskHandle) { task.Block("no remote") }` + "\n")
	for _, f := range findAPI032(review.GoSourceAt("p.go", src, "v1.1.0")) {
		t.Fatalf("a 1.1 pin got a 2.0 finding: %+v", f)
	}
}

// guidanceFindings drops the findings that only say a 1.x type in the
// fragment (TaskHandle, Failure) does not exist in 2.0, which these tests
// do not exercise.
func guidanceFindings(fs []review.Finding) []review.Finding {
	var out []review.Finding
	for _, f := range fs {
		if !strings.Contains(f.Message, "does not compile against") {
			out = append(out, f)
		}
	}
	return out
}
