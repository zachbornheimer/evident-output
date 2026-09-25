package engine

import (
	"strings"
	"testing"
)

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

// TestProgress_NoDoingStreamsEachMilestoneImmediately covers a pure
// Progress/Bytes loop that never calls Doing — a slow download or count
// with no per-item names. Each milestone must stream on the tick that
// crosses it, not on the NEXT milestone or on resolution: deferring a
// milestone waiting for a Doing that will never come leaves the reader
// staring at stale state for a whole milestone, breaking always-show-state.
func TestProgress_NoDoingStreamsEachMilestoneImmediately(t *testing.T) {
	var buf strings.Builder
	out := Init(Config{Isolated: true, Title: "demo", Plain: true, Stdout: &buf, Stderr: &buf})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("download")
	for i := 1; i <= 40; i++ {
		task.Progress(i, 40)
		if i == 4 {
			got := nonEmptyLines(buf.String())
			found := false
			for _, l := range got {
				if strings.Contains(l, "download") && strings.Contains(l, "4/40") {
					found = true
				}
			}
			if !found {
				t.Fatalf("want 4/40 to stream immediately after Progress(4,40), got:\n%s", buf.String())
			}
		}
	}
	task.succeed("")
}
