package engine

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/manifest"
)

// Environment the cross-process helper reads its assignment from. The
// helper runs only when crossProcessRoleEnv is set, so an ordinary test
// run skips it.
const (
	crossProcessRoleEnv  = "EVO_CROSS_PROCESS_ROLE"
	crossProcessStateEnv = "EVO_CROSS_PROCESS_STATE"
	crossProcessDirEnv   = "EVO_CROSS_PROCESS_DIR"
)

// crossProcessDwell is how long each helper stays inside its tracked
// section: long enough that two unsynchronized processes would overlap.
const crossProcessDwell = 300 * time.Millisecond

// crossProcessTask names the Task a helper process runs for role.
func crossProcessTask(role string) string { return "sync " + role }

// sharedTrackedFile is the one tracked file every helper writes.
func sharedTrackedFile(dir string) string { return filepath.Join(dir, "shared.txt") }

// sectionFile is where role's helper records its tracked section.
func sectionFile(dir, role string) string { return filepath.Join(dir, role+".section") }

// section is one helper's tracked interval, in Unix nanoseconds.
type section struct{ enter, exit int64 }

func (s section) overlaps(o section) bool { return s.enter < o.exit && o.enter < s.exit }

// dwellInTrackedSection keeps a helper inside its tracked section for
// crossProcessDwell, or until ctx ends. The dwell is the scenario itself:
// a real process occupying the section another process must not enter.
func dwellInTrackedSection(ctx context.Context) error {
	timer := time.NewTimer(crossProcessDwell)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("dwell in tracked section: %w", ctx.Err())
	}
}

// trackedSection is one helper's Define body: write the shared tracked
// file, dwell, confirm nobody else rewrote it, then record the section's
// interval as a second tracked File.
func trackedSection(ctx context.Context, dir, role string) error {
	shared := sharedTrackedFile(dir)
	if err := File(ctx, FileSpec{Path: shared, Contents: []byte(role)}); err != nil {
		return fmt.Errorf("%s writes shared file: %w", role, err)
	}
	enter := time.Now()
	if err := dwellInTrackedSection(ctx); err != nil {
		return err
	}
	if got, err := os.ReadFile(shared); err != nil || string(got) != role {
		return fmt.Errorf("tracked file rewritten under %s: %q, %v", role, got, err)
	}
	interval := fmt.Sprintf("%d %d", enter.UnixNano(), time.Now().UnixNano())
	if err := File(ctx, FileSpec{Path: sectionFile(dir, role), Contents: []byte(interval)}); err != nil {
		return fmt.Errorf("%s records its section: %w", role, err)
	}
	return nil
}

// TestCrossProcessHelper is one helper process for
// TestCrossProcessSameManifestNeverRacesTrackedState.
func TestCrossProcessHelper(t *testing.T) {
	role := os.Getenv(crossProcessRoleEnv)
	if role == "" {
		t.Skip("runs only as a helper process")
	}
	dir := os.Getenv(crossProcessDirEnv)
	out := newOutput("cross-process", to(io.Discard), plain(), withNoColor(), withStateDir(os.Getenv(crossProcessStateEnv)))
	task := out.Task(crossProcessTask(role))
	task.Define(func(ctx context.Context) error { return trackedSection(ctx, dir, role) })
	if err := task.Wait(); err != nil {
		t.Fatalf("helper %s: %v", role, err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("helper %s close: %v", role, err)
	}
}

// runHelpers starts one helper process per role, all at once, and waits
// for every one to exit cleanly.
func runHelpers(t *testing.T, state, dir string, roles []string) {
	t.Helper()
	helpers := make([]*exec.Cmd, 0, len(roles))
	for _, role := range roles {
		cmd := exec.Command(os.Args[0], "-test.run=^TestCrossProcessHelper$", "-test.count=1", "-test.v")
		cmd.Env = append(os.Environ(),
			crossProcessRoleEnv+"="+role, crossProcessStateEnv+"="+state, crossProcessDirEnv+"="+dir)
		output := &strings.Builder{}
		cmd.Stdout, cmd.Stderr = output, output
		if err := cmd.Start(); err != nil {
			t.Fatalf("start helper %s: %v", role, err)
		}
		t.Cleanup(func() {
			if t.Failed() {
				t.Logf("helper %s output:\n%s", role, output.String())
			}
		})
		helpers = append(helpers, cmd)
	}
	for i, cmd := range helpers {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("helper %s: %v", roles[i], err)
		}
	}
}

func readSection(t *testing.T, dir, role string) section {
	t.Helper()
	raw, err := os.ReadFile(sectionFile(dir, role))
	if err != nil {
		t.Fatalf("read %s section: %v", role, err)
	}
	var s section
	if _, err := fmt.Sscanf(string(raw), "%d %d", &s.enter, &s.exit); err != nil {
		t.Fatalf("parse %s section %q: %v", role, raw, err)
	}
	return s
}

// Two processes reconciling tracked Files in the same manifest namespace
// never interleave: the manifest's exclusive lock, taken before any File
// claims a resource, serializes their tracked sections, and both
// processes' records survive.
func TestCrossProcessSameManifestNeverRacesTrackedState(t *testing.T) {
	if os.Getenv(crossProcessRoleEnv) != "" {
		t.Skip("already a helper process")
	}
	state, dir := t.TempDir(), t.TempDir()
	roles := []string{"alpha", "beta"}
	runHelpers(t, state, dir, roles)

	alpha, beta := readSection(t, dir, "alpha"), readSection(t, dir, "beta")
	if alpha.overlaps(beta) {
		t.Fatalf("tracked sections overlapped across processes: alpha=%+v beta=%+v", alpha, beta)
	}

	store, err := manifest.Open(context.Background(), manifest.Config{StateDir: state}, manifest.NewOSEnvironment())
	if err != nil {
		t.Fatalf("open manifest: %v", err)
	}
	defer func() { _ = store.Close() }()
	keys := newOutput("cross-process", to(io.Discard))
	defer func() { _ = keys.Close() }()
	for _, role := range roles {
		task := keys.Task(crossProcessTask(role))
		keys.mu.Lock()
		key, _, ok := keys.taskManifestKeyLocked(task.id)
		keys.mu.Unlock()
		if !ok {
			t.Fatalf("no manifest key for %s", role)
		}
		if rec, found := store.Task(key); !found || len(rec.Operations) != 2 {
			t.Fatalf("manifest lost %s's tracked state: found=%v record=%+v", role, found, rec)
		}
	}
}
