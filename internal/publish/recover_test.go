package publish

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// testDigest is a tree digest for these tests: every entry's path, and
// every regular file's bytes, in walk order.
func testDigest(_ context.Context, root string) (string, error) {
	h := sha256.New()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		_, _ = fmt.Fprintf(h, "%s\x00", filepath.ToSlash(rel))
		if !d.Type().IsRegular() {
			return nil
		}
		body, err := os.ReadFile(path)
		_, _ = h.Write(body)
		return err
	})
	return hex.EncodeToString(h.Sum(nil)), err
}

// plantTree writes files under root and returns root's digest.
func plantTree(t *testing.T, root string, files map[string]string) string {
	t.Helper()
	for rel, body := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	digest, err := testDigest(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

var (
	originalFiles    = map[string]string{"index.js": "old", "lib/x.js": "x"}
	replacementFiles = map[string]string{"index.js": "new"}
	foreignFiles     = map[string]string{"index.js": "someone else's"}
)

// recoverFixture is a destination beside which an interrupted Replace may
// have left trees, and the digests that Replace planned with.
type recoverFixture struct {
	dest                  string
	original, replacement string
}

func newRecoverFixture(t *testing.T) recoverFixture {
	t.Helper()
	scratch := t.TempDir()
	return recoverFixture{
		dest:        filepath.Join(t.TempDir(), "pkg"),
		original:    plantTree(t, filepath.Join(scratch, "o"), originalFiles),
		replacement: plantTree(t, filepath.Join(scratch, "r"), replacementFiles),
	}
}

// leftover plants files at a fresh staging name owned by dest.
func (f recoverFixture) leftover(t *testing.T, files map[string]string) string {
	t.Helper()
	path := stagingName(f.dest)
	plantTree(t, path, files)
	return path
}

func (f recoverFixture) evidence() Evidence {
	return Evidence{Original: f.original, Replacement: f.replacement, Digest: testDigest}
}

func (f recoverFixture) recover(t *testing.T, ev Evidence) (Recovery, error) {
	t.Helper()
	return Recover(context.Background(), f.dest, ev)
}

func requireDigest(t *testing.T, path, want string) {
	t.Helper()
	if got, err := testDigest(context.Background(), path); err != nil || got != want {
		t.Fatalf("%s digests to %s (%v), want %s", path, got, err, want)
	}
}

func requireLeftovers(t *testing.T, dest string, want ...string) {
	t.Helper()
	got, err := Leftovers(dest)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("Leftovers = %v, want %v", got, want)
	}
}

func TestRecoverIntactRemovesPlannedLeftoversAndKeepsForeignOnes(t *testing.T) {
	f := newRecoverFixture(t)
	plantTree(t, f.dest, originalFiles)
	f.leftover(t, replacementFiles)
	foreign := f.leftover(t, foreignFiles)
	got, err := f.recover(t, f.evidence())
	if err != nil {
		t.Fatal(err)
	}
	if got.Outcome != OutcomeIntact || !slices.Equal(got.Kept, []string{foreign}) {
		t.Fatalf("Recover = %+v, want Intact keeping only %s", got, foreign)
	}
	requireDigest(t, f.dest, f.original)
	requireLeftovers(t, f.dest, foreign)
}

func TestRecoverCompletesAReplacementByRemovingTheOriginal(t *testing.T) {
	f := newRecoverFixture(t)
	plantTree(t, f.dest, replacementFiles)
	f.leftover(t, originalFiles)
	got, err := f.recover(t, f.evidence())
	if err != nil || got.Outcome != OutcomeCompletedReplacement || len(got.Kept) != 0 {
		t.Fatalf("Recover = %+v, %v; want CompletedReplacement keeping nothing", got, err)
	}
	requireDigest(t, f.dest, f.replacement)
	requireLeftovers(t, f.dest)
}

func TestRecoverRestoresTheOriginalOverAMissingDestination(t *testing.T) {
	f := newRecoverFixture(t)
	f.leftover(t, replacementFiles)
	f.leftover(t, originalFiles)
	got, err := f.recover(t, f.evidence())
	if err != nil || got.Outcome != OutcomeRestoredOriginal || len(got.Kept) != 0 {
		t.Fatalf("Recover = %+v, %v; want RestoredOriginal keeping nothing", got, err)
	}
	requireDigest(t, f.dest, f.original)
	requireLeftovers(t, f.dest)
}

func TestRecoverWithoutTheOriginalChangesNothing(t *testing.T) {
	f := newRecoverFixture(t)
	staged := f.leftover(t, replacementFiles)
	foreign := f.leftover(t, foreignFiles)
	got, err := f.recover(t, f.evidence())
	if !errors.Is(err, ErrUnrecoverable) || got.Outcome != OutcomeUnrecoverable {
		t.Fatalf("Recover = %+v, %v; want Unrecoverable", got, err)
	}
	if want := []string{staged, foreign}; !sameSet(got.Kept, want) {
		t.Fatalf("Kept = %v, want every leftover %v", got.Kept, want)
	}
	if _, err := os.Lstat(f.dest); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Unrecoverable created the destination: %v", err)
	}
	requireLeftovers(t, f.dest, staged, foreign)
}

func TestRecoverOfAForeignDestinationChangesNothing(t *testing.T) {
	f := newRecoverFixture(t)
	foreign := plantTree(t, f.dest, foreignFiles)
	original := f.leftover(t, originalFiles)
	got, err := f.recover(t, f.evidence())
	if !errors.Is(err, ErrUnrecoverable) || got.Outcome != OutcomeUnrecoverable || !slices.Equal(got.Kept, []string{original}) {
		t.Fatalf("Recover = %+v, %v; want Unrecoverable keeping %s", got, err, original)
	}
	requireDigest(t, f.dest, foreign)
	requireLeftovers(t, f.dest, original)
}

func TestRecoverWithAnUnknownReplacementKeepsWhatItCannotIdentify(t *testing.T) {
	f := newRecoverFixture(t)
	plantTree(t, f.dest, originalFiles)
	staged := f.leftover(t, replacementFiles)
	f.leftover(t, originalFiles)
	ev := f.evidence()
	ev.Replacement = ""
	got, err := f.recover(t, ev)
	if err != nil || got.Outcome != OutcomeIntact || !slices.Equal(got.Kept, []string{staged}) {
		t.Fatalf("Recover = %+v, %v; want Intact keeping the unidentified %s", got, err, staged)
	}
	requireLeftovers(t, f.dest, staged)
}

func TestRecoverNeverRestoresThroughASymlinkLeftover(t *testing.T) {
	f := newRecoverFixture(t)
	elsewhere := filepath.Join(t.TempDir(), "elsewhere")
	plantTree(t, elsewhere, originalFiles)
	link := stagingName(f.dest)
	if err := os.Symlink(elsewhere, link); err != nil {
		t.Fatal(err)
	}
	got, err := f.recover(t, f.evidence())
	if !errors.Is(err, ErrUnrecoverable) || !slices.Equal(got.Kept, []string{link}) {
		t.Fatalf("Recover = %+v, %v; want Unrecoverable keeping the symlink", got, err)
	}
	if _, err := os.Lstat(f.dest); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Recover published a symlinked leftover: %v", err)
	}
	requireDigest(t, elsewhere, f.original)
}

// A stage whose writer is alive is not a leftover, whatever it digests to.
func TestRecoverKeepsAStageALiveWriterOwns(t *testing.T) {
	f := newRecoverFixture(t)
	plantTree(t, f.dest, originalFiles)
	live := stageTree(t, f.dest, replacementFiles)
	got, err := f.recover(t, f.evidence())
	if err != nil || got.Outcome != OutcomeIntact || !slices.Equal(got.Kept, []string{live.Path()}) {
		t.Fatalf("Recover = %+v, %v; want Intact keeping the live stage %s", got, err, live.Path())
	}
	if err := live.Commit(context.Background(), Guard{}); err != nil {
		t.Fatalf("the live writer's commit after Recover = %v", err)
	}
	requireDigest(t, f.dest, f.replacement)
}

func TestPlanRecoverReportsTheDecisionAndChangesNothing(t *testing.T) {
	f := newRecoverFixture(t)
	staged := f.leftover(t, replacementFiles)
	original := f.leftover(t, originalFiles)
	got, err := PlanRecover(context.Background(), f.dest, f.evidence())
	if err != nil || got.Outcome != OutcomeRestoredOriginal || len(got.Kept) != 0 {
		t.Fatalf("PlanRecover = %+v, %v; want RestoredOriginal keeping nothing", got, err)
	}
	if _, err := os.Lstat(f.dest); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("PlanRecover restored the destination: %v", err)
	}
	requireLeftovers(t, f.dest, staged, original)
}

func sameSet(a, b []string) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}
