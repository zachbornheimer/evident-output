package evo_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// plainTreeLinePrefixes are the connectors a Task's Problem tree is drawn
// with, after the three-space tree indent.
var plainTreeLinePrefixes = []string{"   ├─", "   │", "   └─"}

// TestPlainProjectionShowsContainersAndClosesTrees pins CONTRACT.md §8's
// "one truth, readable rows" for the Plain projection: a Group declared by
// Build keeps its own row with its items nested beneath it, a Problem tree
// ends on the closing connector, an Excluded Task states its reason once,
// and the partial-mutation band names what each operation did in words.
func TestPlainProjectionShowsContainersAndClosesTrees(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "cfg.txt")
	copied := filepath.Join(dir, "out.txt")
	run := executeContract(context.Background(), t,
		evo.Config{Name: "tree", Output: evo.Plain, State: evo.State{Dir: t.TempDir(), AppID: "contract"}},
		func(_ context.Context, r *evo.Run) error {
			write := r.Task("write config")
			write.Define(func(ctx context.Context) error {
				return evo.File(ctx, evo.FileSpec{Path: config, Contents: []byte("v=1\n"), Mode: 0o644})
			})
			r.Task("build").After(write).Define(func(ctx context.Context) error {
				_, err := evo.Exec(ctx, evo.ExecSpec{Argv: []string{"cp", config, copied}, Outputs: []string{copied}})
				return err
			})
			items := r.Group("items")
			items.Build(func(g *evo.Group) {
				for _, name := range []string{"a", "b", "c"} {
					g.Task("item " + name).Define(func(context.Context) error {
						if name == "b" {
							return evo.Exclude("protected", evo.On(name))
						}
						return nil
					})
				}
			})
			r.Task("lint").Define(func(context.Context) error { return evo.Fail("lint failed") })
			return nil
		})
	lines := strings.Split(run.Stderr, "\n")

	t.Run("a Build Group has a row its items nest under", func(t *testing.T) {
		at := indexOfLine(lines, func(l string) bool { return strings.Contains(l, "items") && !strings.HasPrefix(l, " ") })
		if at < 0 {
			t.Fatalf("no unindented row names the Group items:\n%s", run.Stderr)
		}
		after := at
		for _, item := range []string{"item a", "item b", "item c"} {
			row := indexOfLine(lines, func(l string) bool { return strings.Contains(l, item) })
			if row <= after || !strings.HasPrefix(lines[row], "   ") {
				t.Errorf("%q is not nested under its Group row:\n%s", item, run.Stderr)
			}
			after = row
		}
	})

	t.Run("every Problem tree ends on the closing connector", func(t *testing.T) {
		for i, line := range lines {
			continues := i+1 < len(lines) && isTreeLine(lines[i+1])
			if isTreeLine(line) && !continues && !strings.HasPrefix(line, "   └─") {
				t.Errorf("tree ends on %q, want the closing └─ connector:\n%s", line, run.Stderr)
			}
		}
	})

	t.Run("an Excluded Task states its reason once", func(t *testing.T) {
		if n := strings.Count(run.Stderr, "protected"); n != 1 {
			t.Errorf("reason %q printed %d times, want once:\n%s", "protected", n, run.Stderr)
		}
	})

	t.Run("the partial-mutation band names each operation", func(t *testing.T) {
		band := lineContaining(lines, "already mutated:")
		for _, want := range []string{"wrote cfg.txt", "ran cp"} {
			if !strings.Contains(band, want) {
				t.Errorf("band %q does not say %q", band, want)
			}
		}
		if strings.Contains(band, dir) {
			t.Errorf("band %q names the absolute path %s, want the file name", band, dir)
		}
	})
}

func isTreeLine(line string) bool {
	for _, prefix := range plainTreeLinePrefixes {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

func indexOfLine(lines []string, match func(string) bool) int {
	for i, line := range lines {
		if match(line) {
			return i
		}
	}
	return -1
}

func lineContaining(lines []string, substr string) string {
	if at := indexOfLine(lines, func(l string) bool { return strings.Contains(l, substr) }); at >= 0 {
		return lines[at]
	}
	return ""
}
