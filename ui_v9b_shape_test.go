package evo_test

import (
	"bytes"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestUIV9b_PlannedFrameMatchesHTML is the end-result check for the
// dry-run panel in testdata/ui/v9b.html. The HTML still prints "! kept"
// and a "[planned · warned]" band. The 1.x contract is the later decision:
// a policy exclusion is Skipped, and Skipped does not warn, so those two
// HTML details are rewritten before the comparison. Everything else — the
// subject, the three checked rows, the counts, and the planned ledger —
// must match the rendered frame exactly.
func TestUIV9b_PlannedFrameMatchesHTML(t *testing.T) {
	want := plannedFrameFromHTML(t)
	got := renderPlannedFrame(t)
	if got != want {
		t.Fatalf("planned frame\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

func plannedFrameFromHTML(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/ui/v9b.html")
	if err != nil {
		t.Fatal(err)
	}
	section := string(raw)
	start := strings.Index(section, `id="planned"`)
	if start < 0 {
		t.Fatal("planned panel missing")
	}
	section = section[start:]
	end := strings.Index(section, "</section>")
	section = section[:end]
	rowRE := regexp.MustCompile(`(?s)<div class="row[^"]*">(.*?)</div>`)
	tagRE := regexp.MustCompile(`(?s)<span class="tag[^"]*">(.*?)</span>`)
	stripRE := regexp.MustCompile(`<[^>]+>`)
	var lines []string
	for _, match := range rowRE.FindAllStringSubmatch(section, -1) {
		body := match[1]
		indent := ""
		if strings.Contains(match[0], `class="row i1"`) {
			indent = "  "
		}
		body = tagRE.ReplaceAllString(body, "[$1] ")
		text := indent + stripRE.ReplaceAllString(body, "")
		text = strings.ReplaceAll(text, "&nbsp;", "")
		text = strings.ReplaceAll(text, "! kept", "- skipped")
		text = strings.TrimRight(text, " ")
		if text == "[planned · warned]" {
			continue
		}
		lines = append(lines, text)
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n") + "\n"
}

func renderPlannedFrame(t *testing.T) string {
	t.Helper()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true, DryRun: true, Color: evo.ColorNever, Plain: true,
		Subject: "zq prune  ~/Developer/Software-Automation-Holdings/.worktrees/eapp-system-style-contract-heading",
		Stdout:  &buf, Stderr: io.Discard,
	})
	t.Cleanup(func() { _ = out.Close() })

	checkedOut, protected := evo.Reason("checked out"), evo.Reason("protected")
	dirty, unpushed := evo.Reason("dirty"), evo.Reason("unpushed")
	ignoredFiles := evo.Reason("ignored files")

	categories := out.Group("categories")
	branchItems := categories.Group("branches")
	branches := branchItems.Task("branches")
	worktreeItems := categories.Group("worktrees")
	worktrees := worktreeItems.Task("worktrees")
	remoteItems := categories.Group("remote-tracking")
	remotes := remoteItems.Task("remote-tracking")

	skipItems(branchItems, "branch-checked-out", checkedOut, 283)
	skipItems(branchItems, "branch-unpushed", unpushed, 135)
	skipItems(branchItems, "branch-protected", protected, 1)
	commit(branches.Summary("459 checked"), evo.EffectSpec{Verb: evo.EffectDelete, Object: "local tip", Quantity: 40})

	skipItems(worktreeItems, "worktree-dirty", dirty, 163)
	skipItems(worktreeItems, "worktree-unpushed", unpushed, 89)
	skipItems(worktreeItems, "worktree-ignored-files", ignoredFiles, 40)
	commit(worktrees.Summary("294 checked"), evo.EffectSpec{Verb: evo.EffectRemove, Object: "worktree", Quantity: 1})

	commit(remotes.Summary("4 stale refs"), evo.EffectSpec{Verb: evo.EffectDelete, Object: "stale origin/*", Quantity: 4})

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}
