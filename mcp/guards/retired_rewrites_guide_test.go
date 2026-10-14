package guards_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/rules"
)

// guideBacktick matches one backticked token in a migration-guide cell.
var guideBacktick = regexp.MustCompile("`([^`]+)`")

// rewriteSymbol matches a 2.0 spelling a rewrite names: evo.Exclude,
// Task.Verify, task.Writer(). The capture is the final identifier.
var rewriteSymbol = regexp.MustCompile(`\b(?:evo|Task|task)\.([A-Z]\w*)`)

// guideRow is the 2.0 cell of one row of docs/migration/2.0.md.
type guideRow struct {
	replacement string
}

// parseGuideRows reads every table row of the migration guide whose first
// cell lists backticked 1.x names.
func parseGuideRows(t *testing.T) map[string]guideRow {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(moduleRoot(t), "docs", "migration", "2.0.md"))
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]guideRow{}
	for line := range strings.SplitSeq(string(body), "\n") {
		cells := strings.Split(line, "|")
		if len(cells) < 4 || !strings.Contains(cells[1], "`") {
			continue
		}
		row := guideRow{replacement: strings.TrimSpace(cells[2])}
		for _, m := range guideBacktick.FindAllStringSubmatch(cells[1], -1) {
			rows[strings.TrimSuffix(strings.TrimSpace(m[1]), "()")] = row
		}
	}
	if len(rows) == 0 {
		t.Fatal("docs/migration/2.0.md has no table rows")
	}
	return rows
}

// lookupGuideRow finds the guide row for a retired spelling: the exact
// receiver-qualified spelling first ("TaskHandle.Write"), then the bare name
// the guide lists package-level and handle names under.
func lookupGuideRow(rows map[string]guideRow, spelling string) (guideRow, bool) {
	spelling = strings.TrimSuffix(spelling, "(")
	if row, ok := rows[spelling]; ok {
		return row, true
	}
	row, ok := rows[spelling[strings.LastIndex(spelling, ".")+1:]]
	return row, ok
}

// guideSymbols is every identifier the guide's replacement cell names in
// backticks, final-identifier only.
func guideSymbols(row guideRow) map[string]bool {
	symbols := map[string]bool{}
	for _, m := range guideBacktick.FindAllStringSubmatch(row.replacement, -1) {
		for _, id := range regexp.MustCompile(`[A-Z]\w*`).FindAllString(m[1], -1) {
			symbols[id] = true
		}
	}
	return symbols
}

// rewriteSymbols is every 2.0 spelling a rewrite text names. A review
// suggestion "replace <old call> with <new>" names only <new>: the old call
// is the retired spelling being replaced.
func rewriteSymbols(text string) []string {
	if rest, ok := strings.CutPrefix(text, "replace "); ok {
		if _, after, found := strings.Cut(rest, " with "); found {
			text = after
		}
	}
	var symbols []string
	for _, m := range rewriteSymbol.FindAllStringSubmatch(text, -1) {
		symbols = append(symbols, m[1])
	}
	return symbols
}

// assertRewriteAgrees fails when a rewrite of the retired spelling names a
// 2.0 spelling and none of them is one the guide's row for it gives. Prose
// rewrites ("no knob") and guide cells without code are not compared.
func assertRewriteAgrees(t *testing.T, source, retired, rewrite string, rows map[string]guideRow) {
	t.Helper()
	row, ok := lookupGuideRow(rows, retired)
	if !ok {
		return
	}
	want, got := guideSymbols(row), rewriteSymbols(rewrite)
	if len(got) == 0 || len(want) == 0 {
		return
	}
	for _, symbol := range got {
		if want[symbol] {
			return
		}
	}
	t.Errorf("%s rewrites %s to %v, but docs/migration/2.0.md maps it to %q", source, retired, got, row.replacement)
}

// removedTaskCalls is one call shape per removed Task method the review
// detector rewrites; each is checked against the guide row of the same name.
var removedTaskCalls = map[string]string{
	"Block":    `func f(task *evo.TaskHandle) { task.Block("no remote") }`,
	"Blockf":   `func f(task *evo.TaskHandle, n int) { task.Blockf("%d refs", n) }`,
	"Fail":     `func f(task *evo.TaskHandle) { task.Fail("lint failed") }`,
	"Failf":    `func f(task *evo.TaskHandle, n int) { task.Failf("%d bad", n) }`,
	"Skipped":  `func f(task *evo.TaskHandle) { task.Skipped(evo.Reason("protected")) }`,
	"Kept":     `func f(task *evo.TaskHandle, reason string) { task.Kept(reason) }`,
	"Warn":     `func f(task *evo.TaskHandle) { task.Warn("stale") }`,
	"Step":     `func f(task *evo.TaskHandle) { task.Step(1, 3, "file.go") }`,
	"Evidence": `func f(task *evo.TaskHandle) { _ = task.Evidence() }`,
}

// TestRetiredRewritesMatchMigrationGuide keeps docs/migration/2.0.md the one
// source of truth for what replaces a retired name: the retired table, the
// migration rows and the review detector's suggestions must each give the
// replacement the guide gives, so no retired name has two 2.0 spellings.
func TestRetiredRewritesMatchMigrationGuide(t *testing.T) {
	rows := parseGuideRows(t)

	for _, sym := range rules.RetiredSymbols() {
		assertRewriteAgrees(t, "retired table row "+sym.Contract, sym.Contract, sym.Replacement, rows)
	}

	for _, row := range rules.Migrations() {
		if !row.Removed {
			continue
		}
		for _, m := range regexp.MustCompile(`(?:task|evo|Output)\.([A-Z]\w*)`).FindAllStringSubmatch(row.From, -1) {
			assertRewriteAgrees(t, "migration row "+row.From, m[1], row.To, rows)
		}
	}

	src := "package p\nimport evo \"github.com/zachbornheimer/evident-output/v2\"\n"
	for name, body := range removedTaskCalls {
		res := review.GoSource("rewrite.go", src+body+"\n")
		var found bool
		for _, f := range res.Findings {
			if f.RuleID != "API-032" {
				continue
			}
			found = true
			assertRewriteAgrees(t, "review suggestion for "+name, name, f.Suggestion, rows)
		}
		if !found {
			t.Errorf("review reports no API-032 finding for removed call %s", name)
		}
	}
}
