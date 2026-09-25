package engine

import (
	"strings"
	"testing"
)

// TestProgressDoing_PlainMilestoneNamesItsOwnItem is E-119's canonical
// loop, `task.Progress(i, total).Doing(item)`: each milestone pairs its
// count with its item on ONE line, never two. Before this fix, the first
// milestone streamed a bare "1/20" line the instant its count crossed, then
// Doing's item-only follow-up streamed a second "w-01" line right behind
// it — the migration doc promises one paired line
// (`sync  8/40  widget-08`), not two.
func TestProgressDoing_PlainMilestoneNamesItsOwnItem(t *testing.T) {
	var buf strings.Builder
	out := Init(Config{Isolated: true, Title: "demo", Plain: true, Stdout: &buf, Stderr: &buf})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("sync")
	for i := 1; i <= 20; i++ {
		task.Progress(i, 20).Doing("w-%02d", i)
	}
	task.succeed("")

	lines := nonEmptyLines(buf.String())
	var milestoneLines []string
	for _, l := range lines {
		if strings.Contains(l, "sync") && strings.Contains(l, "/20") {
			milestoneLines = append(milestoneLines, l)
		}
	}
	if len(milestoneLines) == 0 {
		t.Fatalf("want at least one paired milestone line, got:\n%s", buf.String())
	}
	first := milestoneLines[0]
	if !strings.Contains(first, "1/20") || !strings.Contains(first, "w-01") {
		t.Fatalf("first milestone must pair the count and its own item on one line, got %q\nfull:\n%s", first, buf.String())
	}
	// Exactly one line ever mentions "1/20" — the duplicate-first-milestone
	// bug streamed it bare, then again item-only.
	count := 0
	for _, l := range lines {
		if strings.Contains(l, "1/20") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("want the 1/20 milestone to appear exactly once, got %d times:\n%s", count, buf.String())
	}
}

// TestTaskDoing_NarratesAfterProgressSeals is the regression for a caller
// that finishes a Progress/Bytes loop and then narrates further steps
// (download, verify, unpack): once the count is sealed (Completed>=Total),
// Doing must go back to being a normal durable narrated step — not stay
// live-only forever just because the Task once reported a count.
func TestTaskDoing_NarratesAfterProgressSeals(t *testing.T) {
	var buf strings.Builder
	out := Init(Config{Isolated: true, Title: "demo", Plain: true, Stdout: &buf, Stderr: &buf})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("install")
	task.Doing("download")
	task.Bytes(400, 400)
	task.Doing("verify checksum")
	task.Doing("unpack")
	task.succeed("")

	got := buf.String()
	if !strings.Contains(got, "verify checksum") {
		t.Fatalf("want a durable line for the post-seal step 'verify checksum', got:\n%s", got)
	}
	if !strings.Contains(got, "unpack") {
		t.Fatalf("want a durable line for the post-seal step 'unpack', got:\n%s", got)
	}
}

// TestPlainProgress_DoingBeforeProgress_PinsShape pins the exact line shape
// of the non-canonical Doing-before-Progress loop order — previously
// unpinned beyond "40/40 appears somewhere" — so a future change cannot
// silently drop or duplicate a milestone in this order either.
func TestPlainProgress_DoingBeforeProgress_PinsShape(t *testing.T) {
	var buf strings.Builder
	out := Init(Config{Isolated: true, Title: "demo", Plain: true, Stdout: &buf, Stderr: &buf})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("sync")
	for i := 1; i <= 40; i++ {
		task.Doing("w-%02d", i)
		task.Progress(i, 40)
	}
	task.succeed("")

	got := buf.String()
	if !strings.Contains(got, "40/40") {
		t.Fatalf("want the final milestone 40/40 to survive, got:\n%s", got)
	}
	lines := nonEmptyLines(got)
	seen40 := 0
	for _, l := range lines {
		if strings.Contains(l, "40/40") {
			seen40++
		}
	}
	if seen40 != 1 {
		t.Fatalf("want the final milestone to appear exactly once, got %d:\n%s", seen40, got)
	}
}

func nonEmptyLines(s string) []string {
	var out []string
	for l := range strings.SplitSeq(s, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}
