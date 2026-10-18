package evo

import (
	"context"
	"io"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/testkit"
)

// ledgerShape is the order a run's rows and ledger rows reach a reader: a
// Task or container row as "row <name>" and a ledger row as "ledger
// <subject>", with glyphs, details, timings and paths dropped. A container
// Plain streams as `◐ name` then `✓ name` shows once, at its first row.
func ledgerShape(text string) []string {
	var shape []string
	for line := range strings.SplitSeq(text, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		trimmed := strings.TrimSpace(timingSuffix.ReplaceAllString(line, ""))
		var entry string
		if rest, ok := strings.CutPrefix(trimmed, "[changed] "); ok {
			name, _, _ := strings.Cut(rest, "  ")
			entry = "ledger " + strings.TrimSpace(name)
		} else {
			fields := strings.SplitN(trimmed, " ", 2)
			if len(fields) < 2 {
				continue
			}
			name, _, _ := strings.Cut(strings.TrimSpace(fields[1]), "  ")
			depth := (len(line) - len(strings.TrimLeft(line, " "))) / 3
			entry = "row " + string(rune('0'+depth)) + name
		}
		if !slices.Contains(shape, entry) || strings.HasPrefix(entry, "ledger ") {
			shape = append(shape, entry)
		}
	}
	return shape
}

var fileSubject = regexp.MustCompile(`\S+\.txt`)

// declareRootFileAfterContainer is one program for both renderers: a
// container, and a root Task that ran After it and writes a file, so its
// File ledger row is named and streams when the Task settles.
func declareRootFileAfterContainer(dir string) func(*Run) {
	return func(out *Run) {
		gen := out.Group("generate")
		gen.Task("write a").Define(func(context.Context) error { return nil })
		gen.Task("write b").Define(func(context.Context) error { return nil })
		out.Task("concat").After(gen).Define(func(ctx context.Context) error {
			return File(ctx, FileSpec{Path: filepath.Join(dir, "all.txt"), Contents: []byte("ab\n")})
		})
	}
}

// TestHumanLedgerRowsFollowTheirTaskAndEndLedgerIsComplete pins that a root
// Task's File ledger row reaches the Human scrollback right after the
// Task's own row, where Plain writes it, and that every change the run made
// is in the final screen: a Task that waits on an open container holds its
// row, and its ledger row waits with it.
func TestHumanLedgerRowsFollowTheirTaskAndEndLedgerIsComplete(t *testing.T) {
	plain, _ := runPlain(t, Config{}, declareRootFileAfterContainer(t.TempDir()))

	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.Height(24), testkit.NoColor())
	out := initRun(Config{Facades: Facades{Stdout: io.Discard, Stderr: io.Discard, Clock: testkit.NewClock(), Terminal: screen}},
		tuning{visibilityDelay: DelayForTest(0), color: colorNever, maxFrameRate: 1_000_000})
	declareRootFileAfterContainer(t.TempDir())(out)
	_ = out.close()

	want, got := ledgerShape(plain), ledgerShape(screen.PersistedText())
	if !slices.Equal(got, want) {
		t.Fatalf("Human writes %q, Plain writes %q\n--- human ---\n%s\n--- plain ---\n%s", got, want, screen.PersistedText(), plain)
	}
	task, ledger := slices.Index(got, "row 0concat"), slices.Index(got, "ledger concat")
	if task < 0 || ledger != task+1 {
		t.Fatalf("the File ledger row is not right after its Task row: %q", got)
	}
	if !fileSubject.MatchString(screen.PersistedText()) {
		t.Fatalf("the final screen names no file change:\n%s", screen.PersistedText())
	}
}
