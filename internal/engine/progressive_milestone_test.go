package engine

import (
	"fmt"
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

// TestPlainProgress_PreludeDoing_ThenCanonicalLoop is the regression for
// the E-119 review's RED repro (r8-red-e119.txt): an ordinary narrated
// Doing that runs before a canonical Progress(...).Doing(...) loop starts
// must not be mistaken for that loop's own first item — no call-order
// signal can tell the two apart (see emitPlainProgressLocked), so a
// counted Task's Doing never forces a durable line at all (reportsCount)
// and every milestone streams its own item-free line instead. The prelude
// step still narrates. The bare milestone tick for 20/20 streams once; the
// loop's own last Doing (w-20) narrates its own separate line right after,
// same as any post-loop step once a count has sealed.
func TestPlainProgress_PreludeDoing_ThenCanonicalLoop(t *testing.T) {
	var buf strings.Builder
	out := Init(Config{Isolated: true, Title: "demo", Plain: true, Stdout: &buf, Stderr: &buf})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("sync")
	task.Doing("reading manifest")
	for i := 1; i <= 20; i++ {
		task.Progress(i, 20).Doing("w-%02d", i)
	}
	task.succeed("")

	got := buf.String()
	if !strings.Contains(got, "reading manifest") {
		t.Fatalf("want the prelude step narrated, got:\n%s", got)
	}
	// Every item but the loop's own last one (w-20) stays live-only: the
	// count seals on the final Progress(20, 20) before its chained Doing
	// runs, so that one narrates normally, same as any post-loop step.
	for i := 1; i < 20; i++ {
		item := fmt.Sprintf("w-%02d", i)
		if strings.Contains(got, item) {
			t.Fatalf("want %q (an open-count item) never shown, got:\n%s", item, got)
		}
	}
	lines := nonEmptyLines(got)
	bareFinal := 0
	for _, l := range lines {
		if strings.Contains(l, "20/20") && !strings.Contains(l, "w-20") {
			bareFinal++
		}
	}
	if bareFinal != 1 {
		t.Fatalf("want the bare final milestone to appear exactly once, got %d:\n%s", bareFinal, got)
	}
	if !strings.Contains(got, "20/20  w-20") {
		t.Fatalf("want the loop's own last Doing to narrate once the count has sealed, got:\n%s", got)
	}
}

// TestPlainProgress_DoingBeforeProgress_NeverPairs pins that the
// Doing-before-Progress order (`task.Doing(item); task.Progress(i, n)`) is
// not a supported pairing shape (only docs/migration/1.1.md's canonical
// `Progress(...).Doing(...)` is): the final milestone still survives
// exactly once, but items are never guessed onto the wrong count.
func TestPlainProgress_DoingBeforeProgress_NeverPairs(t *testing.T) {
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

// TestPlainBytes_DoingPairing_KeepsBytesFormatting pins that a Bytes
// milestone's immediate line still renders as bytes (MB/GB), not a bare
// determinate count, regardless of a Doing chained after it (which stays
// live-only while the count is open).
func TestPlainBytes_DoingPairing_KeepsBytesFormatting(t *testing.T) {
	var buf strings.Builder
	out := Init(Config{Isolated: true, Title: "demo", Plain: true, Stdout: &buf, Stderr: &buf})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("dl")
	task.Bytes(50<<20, 500<<20)
	task.Doing("mirror-a")
	task.succeed("")

	got := buf.String()
	if strings.Contains(got, "52428800") || strings.Contains(got, "524288000") {
		t.Fatalf("want bytes formatting (MB), got raw byte counts:\n%s", got)
	}
	if !strings.Contains(got, "MB") {
		t.Fatalf("want a paired Bytes/Doing line formatted in MB, got:\n%s", got)
	}
}

// TestPlainProgress_NarratesEveryPostLoopStep is the regression for the
// E-119 review's dropped-narration bug under the canonical
// `task.Progress(i, total).Doing(item)` order: once a loop's final tick
// seals the count, every further Doing must narrate — not just the first
// one after the seal (pairsWithMilestone previously took the very next
// post-seal Doing as an item pairing and discarded it).
func TestPlainProgress_NarratesEveryPostLoopStep(t *testing.T) {
	var buf strings.Builder
	out := Init(Config{Isolated: true, Title: "demo", Plain: true, Stdout: &buf, Stderr: &buf})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("mirror")
	for i := 1; i <= 5; i++ {
		task.Progress(i, 5).Doing("w-%d", i)
	}
	task.Doing("verify checksum")
	task.Doing("unpack")
	task.succeed("")

	got := buf.String()
	if !strings.Contains(got, "verify checksum") {
		t.Fatalf("want a durable line for the post-loop step 'verify checksum', got:\n%s", got)
	}
	if !strings.Contains(got, "unpack") {
		t.Fatalf("want a durable line for the post-loop step 'unpack', got:\n%s", got)
	}
}

// TestPlainProgress_FirstTickStreamsImmediately is always-show-state for a
// Task's very first Progress/Bytes tick: it streams its own line the
// instant it happens, the same as every later milestone — never held back
// waiting for a Doing that may not come, which would leave a lone tick
// with no further ticks silent until the task resolves.
func TestPlainProgress_FirstTickStreamsImmediately(t *testing.T) {
	var buf strings.Builder
	out := Init(Config{Isolated: true, Title: "demo", Plain: true, Stdout: &buf, Stderr: &buf})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("download")
	task.Bytes(50<<20, 500<<20)
	if got := buf.String(); !strings.Contains(got, "MB") {
		t.Fatalf("want the first tick to stream immediately, got:\n%s", got)
	}
	task.succeed("")
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
