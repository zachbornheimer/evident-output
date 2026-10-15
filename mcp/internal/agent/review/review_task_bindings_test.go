package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
)

// TestTaskBindingsRecognizeEvoTask: a 2.0 program types its Task as
// *evo.Task, so the receiver-proving detectors (API-032's removed Warn, Done
// and Record*) must see a parameter, a field and a var of that type as a Task
// -- while a same-named method on an unrelated type stays silent.
func TestTaskBindingsRecognizeEvoTask(t *testing.T) {
	cases := map[string]struct{ body, want string }{
		"parameter": {
			`func f(task *evo.Task) { task.Warn("stale") }`,
			`task.Problem("stale")`,
		},
		"field": {
			`type s struct{ scan *evo.Task }
func (x s) f() { x.scan.Done("3 checked") }`,
			`x.scan.Summary("3 checked").Define(`,
		},
		"var": {
			`func f() {
	var task *evo.Task
	task.Step(1, 3, "file.go")
}`,
			`task.Progress(1, 3).Doing("file.go")`,
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got := joinSuggestions(findAPI032(review.GoSource("p.go", evoBody(c.body+"\n"))))
			if !strings.Contains(got, c.want) {
				t.Fatalf("want a suggestion containing %q, got %q", c.want, got)
			}
		})
	}

	lookalike := evoBody(`type recorder struct{}
func (recorder) Warn(msg string) {}
func f(r recorder, task *evo.Task) { r.Warn("x"); task.Summary("ok") }
`)
	if found := findAPI032(review.GoSource("p.go", lookalike)); len(found) != 0 {
		t.Fatalf("lookalike receiver flagged: %+v", found)
	}
}
