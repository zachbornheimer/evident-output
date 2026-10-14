package evo

import (
	"context"
	"io"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/v2/testkit"
)

// finalScreenOrderCeiling lets declareUnrelated's first container wait on its
// second while both hold a slot, regardless of GOMAXPROCS.
const finalScreenOrderCeiling = 2

// timingSuffix is the elapsed time a row may end in.
var timingSuffix = regexp.MustCompile(`\s+—?\s*\d+(\.\d+)?(ms|s|m)$`)

// orderedRows is the order a run's rows reach a reader: one entry per row,
// indented by depth, as "<depth><name>". It keeps the row's name and drops
// its glyph, detail and timing suffix, and stops at the conclusion band.
// Plain streams a container that outlasts its grace as a header (`◐ name`)
// and later a closing verdict row (`✓ name`); the Human final screen shows
// that container once, at the position of its first row, so a name repeated
// at the same depth after its first appearance is folded into the first.
// The members of a streamed container (see streamedContainers) are sorted:
// Plain lists them as they settled and the Human final screen as declared,
// the agreed form CONTRACT §8 states.
func orderedRows(text string, streamed map[string]bool) []string {
	var rows []string
	for line := range strings.SplitSeq(text, "\n") {
		if strings.HasPrefix(line, "[") || strings.TrimSpace(line) == "" {
			if len(rows) > 0 && strings.HasPrefix(line, "[") {
				break
			}
			continue
		}
		depth := (len(line) - len(strings.TrimLeft(line, " "))) / 3
		fields := strings.SplitN(strings.TrimSpace(timingSuffix.ReplaceAllString(line, "")), " ", 2)
		if len(fields) < 2 {
			continue
		}
		name, _, _ := strings.Cut(strings.TrimSpace(fields[1]), "  ")
		row := string(rune('0'+depth)) + name
		if depth == 0 && slices.Contains(rows, row) {
			continue
		}
		rows = append(rows, row)
	}
	return sortStreamedMembers(rows, streamed)
}

// streamedContainers are the names of the root containers Plain wrote as a
// block: the ones whose first row is a `◐` header.
func streamedContainers(plain string) map[string]bool {
	streamed := map[string]bool{}
	for line := range strings.SplitSeq(plain, "\n") {
		if name, ok := strings.CutPrefix(line, "◐ "); ok {
			name, _, _ = strings.Cut(name, "  ")
			streamed[strings.TrimSpace(timingSuffix.ReplaceAllString(name, ""))] = true
		}
	}
	return streamed
}

// sortStreamedMembers sorts each run of member rows (depth 1 and deeper)
// under a root row named in streamed.
func sortStreamedMembers(rows []string, streamed map[string]bool) []string {
	out := slices.Clone(rows)
	for i := 0; i < len(out); {
		if !strings.HasPrefix(out[i], "0") || !streamed[out[i][1:]] {
			i++
			continue
		}
		end := i + 1
		for end < len(out) && !strings.HasPrefix(out[end], "0") {
			end++
		}
		slices.Sort(out[i+1 : end])
		i = end
	}
	return out
}

// declareDependents is one program for both renderers: two containers, the second
// After the first, and a Task After both, so every order is fixed by the graph.
func declareDependents(out *Run) {
	g := out.Group("g")
	g.Task("g1").Define(func(context.Context) error { return nil })
	g.Task("g2").Define(func(context.Context) error { return nil })
	s := out.Sequence("s").After(g)
	s.Task("s1").Define(func(context.Context) error { return nil })
	out.Task("afterBoth").After(g, s).Define(func(context.Context) error { return nil })
}

// declareUnrelated is one program for both renderers: two containers that
// share no edge, declared first then second, where first cannot finish until
// second has. Declaration order and settle order differ, and only settle
// order is what Plain writes.
func declareUnrelated(out *Run) {
	secondDone := make(chan struct{})
	first := out.Group("first")
	first.Task("f1").Define(func(context.Context) error { <-secondDone; return nil })
	second := out.Group("second")
	second.Task("s1").Define(func(context.Context) error { close(secondDone); return nil })
}

// pause is a Define callback that holds its Task Running for d on the real
// clock, so a block outlasts Plain's grace and members settle out of
// declaration order.
func pause(d time.Duration) func(context.Context) error {
	return func(context.Context) error { time.Sleep(d); return nil }
}

// declareStreamedBlock is one program for both renderers on the real clock:
// a Group that outlasts Plain's grace, whose members settle in the reverse of
// declaration order, then a Task After it.
func declareStreamedBlock(out *Run) {
	g := out.Group("pkg")
	g.Task("slowest").Define(pause(700 * time.Millisecond))
	g.Task("middle").Define(pause(100 * time.Millisecond))
	g.Task("fastest").Define(pause(10 * time.Millisecond))
	out.Task("tail").After(g).Define(pause(0))
}

// declareNotStartedDependent is one program for both renderers: a Task After
// an Excluded member of a Group never starts, and must still print below the
// Group that explains why.
func declareNotStartedDependent(out *Run) {
	g := out.Group("eg")
	skipped := g.Task("skipped").Define(func(context.Context) error { return Exclude("not here") })
	g.Task("works").Define(pause(0))
	out.Task("depSkipped").After(skipped).Define(pause(0))
	out.Task("depGroup").After(g).Define(pause(0))
}

// declareChainAfterBlock is one program for both renderers: a chain of
// Tasks After a Group, declared last-link first, so release order differs
// from declaration order.
func declareChainAfterBlock(out *Run) {
	g := out.Group("g")
	g.Task("g1").Define(pause(0))
	g.Task("g2").Define(pause(20 * time.Millisecond))
	second := out.Task("second")
	first := out.Task("first").After(g).Define(pause(0))
	second.After(first).Define(pause(0))
}

// TestHumanFinalScreenOrderMatchesPlain pins that the Human final screen
// lists rows in the order Plain writes them: a container before the Task that
// ran After it, and unrelated containers in the order they settled. It
// compares the ordered rows (see orderedRows for how Plain's streamed
// `◐ name` ... `✓ name` block normalizes to the single container row Human
// shows, and which suffixes are stripped), not the set of rows.
func TestHumanFinalScreenOrderMatchesPlain(t *testing.T) {
	for _, c := range []struct {
		name          string
		declare       func(*Run)
		before, after string
		realClock     bool // the program sleeps, so the block streams in Plain
	}{
		{"dependents", declareDependents, "0g", "0afterBoth", false},
		{"unrelated", declareUnrelated, "0second", "0first", false},
		{"streamed block", declareStreamedBlock, "0pkg", "0tail", true},
		{"not-started dependent", declareNotStartedDependent, "0eg", "0depSkipped", true},
		{"chain released in order", declareChainAfterBlock, "0first", "0second", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			plain, _ := runPlain(t, Config{MaxConcurrency: finalScreenOrderCeiling}, c.declare)

			screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.Height(24), testkit.NoColor())
			facades := Facades{Stdout: io.Discard, Stderr: io.Discard, Terminal: screen}
			if !c.realClock {
				facades.Clock = testkit.NewClock()
			}
			out := initRun(Config{MaxConcurrency: finalScreenOrderCeiling, Facades: facades},
				tuning{visibilityDelay: DelayForTest(0), color: colorNever, maxFrameRate: 1_000_000})
			c.declare(out)
			_ = out.close()

			streamed := streamedContainers(plain)
			want, got := orderedRows(plain, streamed), orderedRows(screen.PersistedText(), streamed)
			if !slices.Equal(got, want) {
				t.Fatalf("Human final screen rows are ordered %q, Plain writes %q\n--- human ---\n%s\n--- plain ---\n%s",
					got, want, screen.PersistedText(), plain)
			}
			if i, j := slices.Index(got, c.before), slices.Index(got, c.after); i < 0 || j < i {
				t.Fatalf("%q is not above %q: %q", c.before, c.after, got)
			}
		})
	}
}

// declareRootGroupBeforeSlowTask is one program for both renderers on the real
// clock: a root Group that settles at once (its rows write whole inside the
// grace), then a free root Task that settles 300 ms later.
func declareRootGroupBeforeSlowTask(out *Run) {
	g := out.Group("fast group")
	g.Task("f1").Define(pause(0))
	g.Task("f2").Define(pause(0))
	out.Task("slow task").Define(pause(300 * time.Millisecond))
}

// declareFreeTaskBetweenGroups is one program for both renderers on the real
// clock: a free Task that settles at once, a Group that settles after it, and
// a free Task that settles after the Group.
func declareFreeTaskBetweenGroups(out *Run) {
	out.Task("early").Define(pause(0))
	g := out.Group("middle")
	g.Task("m1").Define(pause(100 * time.Millisecond))
	out.Task("late").Define(pause(300 * time.Millisecond))
}

// declareStreamedBlockBeforeSlowTask is one program for both renderers on the
// real clock: a Group that outlasts the grace and settles at 700 ms, then a
// free Task that settles at 1 s. Plain wrote the block's header at 500 ms.
func declareStreamedBlockBeforeSlowTask(out *Run) {
	g := out.Group("pkg")
	g.Task("p1").Define(pause(700 * time.Millisecond))
	out.Task("late").Define(pause(time.Second))
}

// TestHumanFinalScreenMatchesPlainDurableOrder pins that a free root row and a
// root collection appear on the Human final screen in the order they became
// durable in Plain, whichever of them settled first. The terminal keeps
// collections in its region until Finish, so a row that settles after one waits
// there and Finish writes both in that order, instead of the row printing above
// the collection Plain wrote before it.
func TestHumanFinalScreenMatchesPlainDurableOrder(t *testing.T) {
	for _, c := range []struct {
		name    string
		declare func(*Run)
		order   []string // the root rows, top to bottom
	}{
		{"a free Task settling after a Group that settled at once", declareRootGroupBeforeSlowTask, []string{"0fast group", "0slow task"}},
		{"a Group between two free Tasks", declareFreeTaskBetweenGroups, []string{"0early", "0middle", "0late"}},
		{"a Group that streamed in Plain before a later free Task", declareStreamedBlockBeforeSlowTask, []string{"0pkg", "0late"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			plain, _ := runPlain(t, Config{}, c.declare)

			screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.Height(24), testkit.NoColor())
			out := initRun(Config{Facades: Facades{Stdout: io.Discard, Stderr: io.Discard, Terminal: screen}},
				tuning{visibilityDelay: DelayForTest(0), color: colorNever, maxFrameRate: 1_000_000})
			c.declare(out)
			_ = out.close()

			streamed := streamedContainers(plain)
			want, got := orderedRows(plain, streamed), orderedRows(screen.PersistedText(), streamed)
			if !slices.Equal(got, want) {
				t.Fatalf("Human final screen rows are ordered %q, Plain writes %q\n--- human ---\n%s\n--- plain ---\n%s",
					got, want, screen.PersistedText(), plain)
			}
			var roots []string
			for _, row := range got {
				if strings.HasPrefix(row, "0") {
					roots = append(roots, row)
				}
			}
			if !slices.Equal(roots, c.order) {
				t.Fatalf("root rows are ordered %q, want %q", roots, c.order)
			}
		})
	}
}
