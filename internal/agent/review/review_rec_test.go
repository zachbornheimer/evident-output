package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// Positional quantity-first Delete is a removed mutation shape. API-032 must
// emit mechanically applicable one-liners that name Config fields and the
// 1.1 replacement, Define + evo.Effect with EffectSpec.Quantity.
func TestAPI032_RepoRetireConfigAndDeleteShape(t *testing.T) {
	src := `package p
import (
	"bytes"
	evo "github.com/zachbornheimer/evident-output"
)
func f(task *evo.TaskHandle, n int) {
	var buf bytes.Buffer
	evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain()}})
	task.Delete(n, "local tip")
}
`
	res := review.GoSource("clean.go", src)
	found := findAPI032(res)
	if len(found) == 0 {
		t.Fatalf("want API-032 for Options/To/Plain/quantity-first Delete, got no findings")
	}
	joined := joinSuggestions(found)
	want := `with task.Define(func(ctx context.Context) error { return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectDelete, Object: "local tip", Quantity: n}, fn) })`
	if !strings.Contains(joined, want) {
		t.Fatalf("suggestion must rewrite to Define + evo.Effect:\nwant %s\ngot  %q", want, joined)
	}
	if strings.Contains(joined, "Affected") {
		t.Fatalf("suggestion must not teach the removed evo.Affected, got %q", joined)
	}
	if !strings.Contains(joined, "Stdout:") || !strings.Contains(joined, "Plain:") {
		t.Fatalf("suggestion must name Config fields Stdout and Plain, not Option funcs, got %q", joined)
	}
	if strings.Contains(joined, "evo.To(") && strings.Contains(joined, "with evo.To(") {
		t.Fatalf("replacement must not keep evo.To, got %q", joined)
	}
}

func TestAPI032_OldRemoveSkipStartPhaseMainWith(t *testing.T) { // MainWith: removed in 1.0, still detected as a fixture of old code
	src := `package main
import evo "github.com/zachbornheimer/evident-output"
func main() {
	out := evo.Init(evo.Config{Title: "t", Isolated: true})
	evo.MainWith(out, run) // MainWith: removed in 1.0, fixture pins the detector still catches old call sites
}
func run(out *evo.Output) error {
	task := out.Task("worktrees", evo.StartPhase("scanning worktrees"))
	_ = out.Task("scan", evo.ID("scan.run"))
	task.Remove(n, "worktree")
	task.Skip("skipped")
	evo.Task("scanning %s", name)
	return nil
}
`
	res := review.GoSource("extra.go", src)
	found := findAPI032(res)
	joined := joinSuggestions(found)
	cases := []string{
		`evo.EffectRemove, Object: "worktree", Quantity: n}`,
		`task.Skipped(evo.Reason("skipped")`,
		`.Doing("scanning worktrees")`,
		`with out.Task("scan")`,
		"out.Run(run)",
		`evo.Task(fmt.Sprintf("scanning %s", name))`,
	}
	for _, want := range cases {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in suggestions %q", want, joined)
		}
	}
	if len(found) == 0 {
		t.Fatalf("want API-032 for Remove/Skip/StartPhase/MainWith (removed in 1.0)/Task-printf, got none")
	}
}

func TestAPI032_StandaloneOptionFuncs(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
func f(r io.Reader, w io.Writer, d time.Duration) {
	_ = evo.NoColor()
	_ = evo.Stdin(r)
	_ = evo.DryRun()
	_ = evo.VisibilityDelay(d)
	_ = evo.Diagnostics(w)
}
`
	res := review.GoSource("opts.go", src)
	joined := joinSuggestions(findAPI032(res))
	if strings.Contains(joined, "evo.Delay") {
		t.Fatalf("VisibilityDelay must not name unexported Delay, got %q", joined)
	}
	for _, want := range []string{
		"Color: evo.ColorNever",
		"Stdin: r",
		"DryRun: true",
		"VisibilityDelay: &d",
		"Stderr: w",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in suggestions %q", want, joined)
		}
	}
}

func TestAPI032_TestingTSkipNotFlagged(t *testing.T) {
	src := `package p_test
import (
	"testing"
	evo "github.com/zachbornheimer/evident-output"
)
func TestX(t *testing.T) {
	evo.Init(evo.Config{Title: "t"})
	t.Skip("not in this environment")
}
`
	res := review.GoSource("p_test.go", src)
	for _, f := range findAPI032(res) {
		if strings.Contains(f.Suggestion, "Skip") || strings.Contains(f.Message, "Skip") {
			t.Fatalf("testing.T.Skip must not be API-032: %+v", f)
		}
	}
}

func TestAPI032_CurrentRecSurfaceNotFlagged(t *testing.T) {
	src := `package p
import (
	"bytes"
	evo "github.com/zachbornheimer/evident-output"
)
func f(task *evo.TaskHandle, n int, reason evo.TaxonomyReason) {
	var buf bytes.Buffer
	evo.Init(evo.Config{Stdout: &buf, Plain: true, Color: evo.ColorNever, DryRun: true})
	task.Define(func(ctx context.Context) error {
		return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectDelete, Object: "local tip", Quantity: n}, removeTip)
	})
	task.Skipped(reason)
	task.Doing("scanning worktrees")
	evo.Task("branches")
}
`
	res := review.GoSource("current.go", src)
	if found := findAPI032(res); len(found) != 0 {
		t.Fatalf("current rec surface must not be API-032: %+v", found)
	}
}

func TestAPI032_NonEvoWriteWithThreeArgsNotFlagged(t *testing.T) {
	src := `package p
import (
	evo "github.com/zachbornheimer/evident-output"
	"example.com/cache"
)
func stamp(path string, task *evo.TaskHandle, n int) {
	_ = cache.Write(path, cache.Stamp{Reason: "skip"}, func() error { return nil })
	task.Write(n, "dst")
}
`
	res := review.GoSource("stamp.go", src)
	for _, f := range findAPI032(res) {
		if strings.Contains(f.Suggestion, "cache.Write") || strings.Contains(f.Message, "cache.Write") {
			t.Fatalf("package Write must not be API-032: %+v", f)
		}
	}
	joined := joinSuggestions(findAPI032(res))
	if !strings.Contains(joined, "task.Write") {
		t.Fatalf("task.Write(n, object) must still flag, got %q", joined)
	}
}

func TestAPI032_PositionalQuantityIsOldShape(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
func f(task *evo.TaskHandle, n int) {
	task.Delete(1, "local tip")
	task.Delete(n, "worktree")
}
`
	res := review.GoSource("old.go", src)
	found := findAPI032(res)
	if len(found) < 2 {
		t.Fatalf("want API-032 on both quantity-first Delete calls, got %+v", found)
	}
	joined := joinSuggestions(found)
	if !strings.Contains(joined, `Object: "local tip", Quantity: 1}`) || !strings.Contains(joined, `Object: "worktree", Quantity: n}`) {
		t.Fatalf("suggestion must carry each object and quantity into EffectSpec, got %q", joined)
	}
}

// TestAPI032_LegacyObjectFirstMutationVerbIsRemoved pins ZYS-950: the
// 1.0 object-first spelling Delete(object, fn, evo.Affected(n)) was removed
// in 1.1. API-032 rewrites it to Define + evo.Effect, carrying the callback
// body over with the context-taking signature Effect requires.
func TestAPI032_LegacyObjectFirstMutationVerbIsRemoved(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
func f(task *evo.TaskHandle, n int) {
	task.Delete("local tip", func() error { return removeTip() }, evo.Affected(n))
	task.Push("remote ref", nil)
}
`
	res := review.GoSource("legacy.go", src)
	joined := joinSuggestions(findAPI032(res))
	want := `with task.Define(func(ctx context.Context) error { return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectDelete, Object: "local tip", Quantity: n}, func(context.Context) error { return removeTip() }) })`
	if !strings.Contains(joined, want) {
		t.Fatalf("legacy Delete must rewrite to Define + evo.Effect:\nwant %s\ngot  %q", want, joined)
	}
	if !strings.Contains(joined, `evo.EffectPush, Object: "remote ref", Quantity: 1}, fn)`) {
		t.Fatalf("legacy Push with a nil callback must still rewrite to evo.Effect, got %q", joined)
	}
}

// TestAPI032_LegacyWriteRewritesToFile pins ZYS-950's File half: there is
// no EffectWrite, so the removed Task.Write routes to evo.File.
func TestAPI032_LegacyWriteRewritesToFile(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
func f(task *evo.TaskHandle) {
	task.Write("config.toml", func() error { return save() })
}
`
	res := review.GoSource("write.go", src)
	joined := joinSuggestions(findAPI032(res))
	if !strings.Contains(joined, `evo.File(ctx, evo.FileSpec{Path: "config.toml", Contents: data})`) {
		t.Fatalf("legacy Write must rewrite to evo.File, got %q", joined)
	}
	if strings.Contains(joined, "EffectWrite") {
		t.Fatalf("EffectWrite does not exist; got %q", joined)
	}
}

// TestAPI032_NonEvoVerbLookalikesNotFlagged guards the legacy-verb
// detector's blast radius: same-named methods on non-evo values whose
// second argument is not a work callback stay silent.
func TestAPI032_NonEvoVerbLookalikesNotFlagged(t *testing.T) {
	src := `package p
import (
	"net/http"
	evo "github.com/zachbornheimer/evident-output"
)
func f(h http.Header, m map[string]int, ct string, _ *evo.TaskHandle) {
	h.Add("Content-Type", ct)
	h.Del("Accept")
	delete(m, "k")
}
`
	res := review.GoSource("lookalike.go", src)
	if found := findAPI032(res); len(found) != 0 {
		t.Fatalf("non-evo lookalikes must not be API-032: %+v", found)
	}
}

func TestAPI032_RetiredCollectionConstructorIsSuperseded(t *testing.T) {
	ctor := "Display" + "Group"
	src := "package p\nimport evo \"github.com/zachbornheimer/evident-output\"\nfunc f(out *evo.Output) {\n\t_ = out." + ctor + "(\"run\")\n\t_ = evo." + ctor + "(\"worktrees\")\n}\n"
	res := review.GoSource("group.go", src)
	found := findAPI032(res)
	if len(found) == 0 {
		t.Fatalf("want API-032 for retired collection constructor, got none")
	}
	joined := joinSuggestions(found)
	if !strings.Contains(joined, "Group") {
		t.Fatalf("suggestion must name Group, got %q", joined)
	}
}

func joinSuggestions(fs []review.Finding) string {
	var b strings.Builder
	for _, f := range fs {
		b.WriteString(f.Suggestion)
		b.WriteByte('\n')
		b.WriteString(f.Message)
		b.WriteByte('\n')
	}
	return b.String()
}
