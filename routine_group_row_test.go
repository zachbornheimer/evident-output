package evo_test

import (
	"context"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
	"github.com/zachbornheimer/evident-output/v2/testkit"
)

// TestRoutineGroupKeepsItsRowInEveryProjection pins CONTRACT section 8: collapse folds a routine container's
// members under the container's own row and never drops the row. A Group whose Tasks all succeed shows its
// row with each Task indented beneath it in Human and Plain, as evo.run nests them under the collection.
func TestRoutineGroupKeepsItsRowInEveryProjection(t *testing.T) {
	body := func(_ context.Context, r *evo.Run) error {
		installs := r.Group("install")
		installs.Task("a").Define(func(context.Context) error { return nil })
		installs.Task("b").Define(func(context.Context) error { return nil })
		return nil
	}

	t.Run("Plain", func(t *testing.T) {
		run := executeContract(context.Background(), t, evo.Config{Name: "qs", Output: evo.Plain}, body)
		assertGroupRowOwnsItsTasks(t, run.Text())
	})
	t.Run("Human", func(t *testing.T) {
		screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
		run := executeContract(context.Background(), t, evo.Config{Name: "qs", Facades: evo.Facades{Terminal: screen}}, body)
		assertGroupRowOwnsItsTasks(t, run.Text()+screen.PersistedText())
	})
	t.Run("JSON", func(t *testing.T) {
		run := executeContract(context.Background(), t, evo.Config{Name: "qs", Output: evo.JSON}, body)
		data, _ := runDocument(t, run.Stdout)["data"].(map[string]any)
		collections, _ := data["collections"].([]any)
		if len(collections) != 1 {
			t.Fatalf("data.collections = %v, want the install Group", data["collections"])
		}
		group, _ := collections[0].(map[string]any)
		if children, _ := group["children"].([]any); group["name"] != "install" || len(children) != 2 {
			t.Errorf("collection = %v, want install holding a and b", group)
		}
	})
}

// assertGroupRowOwnsItsTasks fails unless text has an "install" row followed by "a" and "b" rows indented
// deeper than it.
func assertGroupRowOwnsItsTasks(t *testing.T, text string) {
	t.Helper()
	indent := map[string]int{}
	for line := range strings.SplitSeq(text, "\n") {
		trimmed := strings.TrimLeft(line, " ")
		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			continue
		}
		switch name := fields[1]; name {
		case "install", "a", "b":
			indent[name] = len(line) - len(trimmed)
		}
	}
	group, hasGroup := indent["install"]
	if !hasGroup {
		t.Fatalf("no row for the install Group; its Tasks are flattened to the top:\n%s", text)
	}
	for _, task := range []string{"a", "b"} {
		if got, ok := indent[task]; !ok || got <= group {
			t.Errorf("Task %s is at indent %d (present %v), want it nested under the install row at %d:\n%s", task, got, ok, group, text)
		}
	}
}
