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
// must not be mistaken for that loop's own first pairing Doing — no call-
// order signal can tell the two apart (see emitPlainProgressLocked), so
// only the canonical order pairs at all. Each milestone must pair with its
// OWN item, not the previous one, and the final count must print exactly
// once.
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
	// Every milestone must pair with its OWN item — the exact RED defect
	// was every milestone naming the PREVIOUS item (2/20 named w-01, 4/20
	// named w-03, ...).
	// The final milestone (20/20) is the documented exception: its own
	// chained Doing goes unshown, already covered by
	// TestProgressDoing_FinalMilestoneDoingNoOrphanLine.
	for i := 2; i < 20; i += 2 {
		want := fmt.Sprintf("%d/20  w-%02d", i, i)
		if !strings.Contains(got, want) {
			t.Fatalf("want milestone %d paired with its own item (%q), got:\n%s", i, want, got)
		}
	}
	lines := nonEmptyLines(got)
	seenFinal := 0
	for _, l := range lines {
		if strings.Contains(l, "20/20") {
			seenFinal++
		}
	}
	if seenFinal != 1 {
		t.Fatalf("want the final milestone to appear exactly once, got %d:\n%s", seenFinal, got)
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

// TestPlainBytes_DoingPairing_KeepsBytesFormatting is the regression for
// owedMilestone dropping Progress.Kind: a Bytes milestone that defers to a
// paired Doing must still render as bytes (MB/GB) on the paired line, not
// silently fall back to a bare determinate count.
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

// TestPlainProgress_FirstTickIsDeferredUntilNextEvent pins the documented
// 1.1 tradeoff (docs/migration/1.1.md "first progress tick is deferred"):
// unlike 1.0, a Task's very first Progress/Bytes tick does not stream its
// own line immediately — it waits for the paired Doing (or, with none
// coming, the next milestone or resolution) so the canonical
// Progress(...).Doing(...) chain can print the count and the item on one
// line instead of two. A lone first tick with nothing narrating it and no
// further ticks stays silent until the task resolves.
func TestPlainProgress_FirstTickIsDeferredUntilNextEvent(t *testing.T) {
	var buf strings.Builder
	out := Init(Config{Isolated: true, Title: "demo", Plain: true, Stdout: &buf, Stderr: &buf})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("download")
	task.Bytes(50<<20, 500<<20)
	if got := buf.String(); got != "" {
		t.Fatalf("want the first tick deferred (no line yet), got:\n%s", got)
	}
	task.succeed("")
	if got := buf.String(); !strings.Contains(got, "MB") {
		t.Fatalf("want the deferred first tick to flush at resolution, got:\n%s", got)
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
