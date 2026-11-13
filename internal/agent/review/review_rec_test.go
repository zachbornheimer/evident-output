package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// repo-retire / zq still spell Init with Config.Options and mutation verbs as
// Delete(object, call, Affected(n)). API-032 must emit mechanically
// applicable one-liners that name Config fields and Delete(n, object).
func TestAPI032_RepoRetireConfigAndDeleteShape(t *testing.T) {
	src := `package p
import (
	"bytes"
	evo "github.com/zachbornheimer/evident-output"
)
func f(task *evo.TaskHandle, n int) {
	var buf bytes.Buffer
	evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain()}})
	_ = task.Delete("local tip", nil, evo.Affected(n))
}
`
	res := review.GoSource("clean.go", src)
	found := findAPI032(res)
	if len(found) == 0 {
		t.Fatalf("want API-032 for Options/To/Plain/Delete/Affected, got no findings")
	}
	joined := joinSuggestions(found)
	if !strings.Contains(joined, `Delete(n, "local tip")`) {
		t.Fatalf("suggestion must name Delete(n, \"local tip\"), got %q", joined)
	}
	if !strings.Contains(joined, "Stdout:") || !strings.Contains(joined, "Plain:") {
		t.Fatalf("suggestion must name Config fields Stdout and Plain, not Option funcs, got %q", joined)
	}
	if strings.Contains(joined, "evo.To(") && strings.Contains(joined, "with evo.To(") {
		t.Fatalf("replacement must not keep evo.To, got %q", joined)
	}
}

func TestAPI032_OldRemoveSkipStartPhaseMainWith(t *testing.T) {
	src := `package main
import evo "github.com/zachbornheimer/evident-output"
func main() {
	out := evo.Init(evo.Config{Title: "t", Isolated: true})
	evo.MainWith(out, run)
}
func run(out *evo.Output) error {
	task := out.Task("worktrees", evo.StartPhase("scanning worktrees"))
	_ = out.Task("scan", evo.ID("scan.run"))
	_ = task.Remove("worktree", nil, evo.Affected(n))
	task.Skip("skipped")
	evo.Task("scanning %s", name)
	return nil
}
`
	res := review.GoSource("extra.go", src)
	found := findAPI032(res)
	joined := joinSuggestions(found)
	cases := []string{
		`Remove(n, "worktree")`,
		`task.Skipped(evo.Reason("skipped"), "skipped")`,
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
		t.Fatalf("want API-032 for Remove/Skip/StartPhase/MainWith/Task-printf, got none")
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
	task.Delete(n, "local tip")
	task.Remove(n, "worktree")
	task.Skipped(reason, "tip")
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
func stamp(path string) {
	_ = cache.Write(a.cache, path, cache.Stamp{Reason: "skip"})
	_ = task.Write("dst", func() error { return nil }, evo.Affected(1))
}
`
	res := review.GoSource("stamp.go", src)
	for _, f := range findAPI032(res) {
		if strings.Contains(f.Suggestion, "cache.Write") || strings.Contains(f.Message, "Write (object, call") && strings.Contains(f.Suggestion, "cache.Write") {
			t.Fatalf("package Write must not be API-032: %+v", f)
		}
	}
	joined := joinSuggestions(findAPI032(res))
	if !strings.Contains(joined, "task.Write") && !strings.Contains(joined, "Affected") {
		t.Fatalf("task.Write old shape / evo.Affected must still flag, got %q", joined)
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
