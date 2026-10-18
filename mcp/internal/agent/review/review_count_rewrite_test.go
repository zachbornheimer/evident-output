package review_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
)

// countRewrite is the text the evo.Count(count, unit) rewrite produces for
// a program that passes the call to Task.Problem.
func countRewrite(t *testing.T, count, unit string) (finding review.Finding, found bool) {
	t.Helper()
	src := fmt.Sprintf(`package main

import (
	"context"

	"github.com/zachbornheimer/evident-output"
)

func run(ctx context.Context) error {
	task := evo.Task("tags")
	task.Define(func(ctx context.Context) error {
		return task.Fail("stale", evo.Count(%s, %s))
	})
	return nil
}
`, count, unit)
	for _, f := range review.GoSource("main.go", src).Findings {
		if strings.Contains(f.Message, "evo.Count was removed") {
			return f, true
		}
	}
	return review.Finding{}, false
}

// TestCountRewritePluralizes pins that the Count rewrite renders the unit the
// way v1 did and the way Effect does: plural unless the count is one.
func TestCountRewritePluralizes(t *testing.T) {
	cases := []struct{ count, unit, want string }{
		{"2", `"tag"`, `evo.Detail("2 tags")`},
		{"1", `"tag"`, `evo.Detail("1 tag")`},
		{"0", `"tag"`, `evo.Detail("0 tags")`},
		{"3", `"branch"`, `evo.Detail("3 branches")`},
		{"2", `"entry"`, `evo.Detail("2 entries")`},
		{"2", `"child"`, `evo.Detail("2 children")`},
		{"4", `"stale local branch"`, `evo.Detail("4 stale local branches")`},
		{"2", `"origin/*"`, `evo.Detail("2 origin/*")`},
	}
	for _, c := range cases {
		t.Run(c.count+" "+c.unit, func(t *testing.T) {
			f, ok := countRewrite(t, c.count, c.unit)
			if !ok || !strings.Contains(f.Suggestion, c.want) {
				t.Fatalf("Count(%s, %s) suggestion = %q, want it to contain %s", c.count, c.unit, f.Suggestion, c.want)
			}
		})
	}
}

// TestCountRewriteLeavesRuntimeCountsToTheAuthor pins that a count or unit
// only known at run time gets guidance and no edit: the rewrite cannot choose
// the plural, and "2 tag" would misreport what v1 showed.
func TestCountRewriteLeavesRuntimeCountsToTheAuthor(t *testing.T) {
	for _, c := range [][2]string{{"len(os.Args)", `"tag"`}, {"2", "unit"}} {
		f, ok := countRewrite(t, c[0], c[1])
		if !ok {
			t.Fatalf("Count(%s, %s) draws no finding", c[0], c[1])
		}
		if strings.Contains(f.Suggestion, "evo.Detail(") && strings.HasPrefix(f.Suggestion, "replace ") {
			t.Errorf("Count(%s, %s) is rewritten without knowing the plural: %q", c[0], c[1], f.Suggestion)
		}
	}
}

// TestCountRewriteMatchesEffectPluralization holds the rewrite's plural to
// what 2.0 renders for the same quantity and object, so the two never drift.
func TestCountRewriteMatchesEffectPluralization(t *testing.T) {
	units := []string{"tag", "branch", "entry", "child", "index", "box", "day", "stale origin/*", "package in .venv", "logs", "class"}
	for _, unit := range units {
		for _, quantity := range []int{1, 2, 8} {
			var buf bytes.Buffer
			evo.Execute(context.Background(), evo.Config{Output: evo.Plain, Facades: evo.Facades{Stderr: &buf}}, func(ctx context.Context, r *evo.Run) error {
				r.Task("cleanup").Define(func(ctx context.Context) error {
					return evo.Effect(ctx, evo.EffectSpec{Verb: evo.Delete, Object: unit, Quantity: quantity}, func(context.Context) error { return nil })
				})
				return nil
			})
			f, ok := countRewrite(t, fmt.Sprint(quantity), fmt.Sprintf("%q", unit))
			if !ok {
				t.Fatalf("Count(%d, %q) draws no finding", quantity, unit)
			}
			text := strings.TrimSuffix(strings.TrimPrefix(f.Suggestion[strings.Index(f.Suggestion, `evo.Detail("`)+len(`evo.Detail("`):], ""), "")
			text = text[:strings.Index(text, `")`)]
			if !strings.Contains(buf.String(), text) {
				t.Errorf("Count(%d, %q) rewrites to %q, but Effect renders:\n%s", quantity, unit, text, buf.String())
			}
		}
	}
}
