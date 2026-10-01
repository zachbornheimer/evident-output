package tree_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/internal/publish"
)

// pruneReplaceFixture is a destination holding tree "old", its planned
// digest, and an archive for tree "new".
type pruneReplaceFixture struct {
	parent, dest, expected string
	next                   evo.Tree
}

func newPruneReplaceFixture(t *testing.T) pruneReplaceFixture {
	t.Helper()
	parent := t.TempDir()
	dest := filepath.Join(parent, "pkg")
	plant(t, dest, map[string]string{"index.js": "old", "lib/x.js": "x"})
	next := evo.Tree{Path: dest, Content: evo.Extract{File: archive(t, t.TempDir(), "next.tgz", map[string]string{"index.js": "new"})}}
	return pruneReplaceFixture{parent: parent, dest: dest, expected: pruneChecksum(t, dest), next: next}
}

func (f pruneReplaceFixture) replace(t *testing.T) error {
	t.Helper()
	return contractRun(t, evo.Config{}, func(ctx context.Context) error { return f.next.Replace(ctx, f.expected) })
}

func TestPrune_ReplaceCommitsWhenTheDestinationIsUnchanged(t *testing.T) {
	f := newPruneReplaceFixture(t)
	if err := f.replace(t); err != nil {
		t.Fatalf("Replace = %v", err)
	}
	if got := onDisk(t, f.dest); !equalFiles(got, map[string]string{"index.js": "new"}) {
		t.Fatalf("destination = %v, want the new tree only", got)
	}
	if names := advNames(t, f.parent); !slices.Equal(names, []string{"pkg"}) {
		t.Fatalf("Replace left %v in the parent; the old tree must be gone once the new one verified", names)
	}
}

func TestPrune_ReplaceOfAnEditedDestinationIsTreeChangedAndPreservesTheEdit(t *testing.T) {
	f := newPruneReplaceFixture(t)
	plant(t, f.dest, map[string]string{"index.js": "local edit"})
	if err := f.replace(t); !errors.Is(err, evo.ErrTreeChanged) {
		t.Fatalf("Replace after a local edit = %v, want ErrTreeChanged", err)
	}
	if got := onDisk(t, f.dest); got["index.js"] != "local edit" || got["lib/x.js"] != "x" {
		t.Fatalf("destination = %v, want the edited tree untouched", got)
	}
}

// The edit lands while Replace waits for the destination's coordination:
// after the caller read expected, before the commit. Only a re-check inside
// the critical section can catch it.
func TestPrune_ReplaceRechecksInsideTheCriticalSection(t *testing.T) {
	f := newPruneReplaceFixture(t)
	hold, err := publish.Lock(context.Background(), f.dest)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- f.replace(t) }()
	plant(t, f.dest, map[string]string{"index.js": "concurrent edit"})
	if err := hold.Release(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, evo.ErrTreeChanged) {
		t.Fatalf("Replace across a concurrent edit = %v, want ErrTreeChanged", err)
	}
	if got := onDisk(t, f.dest); got["index.js"] != "concurrent edit" {
		t.Fatalf("destination = %v, want the concurrent edit preserved", got)
	}
	if names := advNames(t, f.parent); !slices.Equal(names, []string{"pkg"}) {
		t.Fatalf("refused Replace left %v in the parent", names)
	}
}

func TestPrune_ReplaceOfAMissingDestinationIsTreeChanged(t *testing.T) {
	f := newPruneReplaceFixture(t)
	if err := os.RemoveAll(f.dest); err != nil {
		t.Fatal(err)
	}
	if err := f.replace(t); !errors.Is(err, evo.ErrTreeChanged) {
		t.Fatalf("Replace of a vanished destination = %v, want ErrTreeChanged", err)
	}
	if _, err := os.Lstat(f.dest); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("refused Replace created the destination: %v", err)
	}
}

// pruneMustHold takes dest's coordination without waiting.
func pruneMustHold(t *testing.T, dest string) *publish.Hold {
	t.Helper()
	hold, ok, err := publish.TryLock(dest)
	if err != nil || !ok {
		t.Fatalf("TryLock(%s) = %v, %v; want it free", dest, ok, err)
	}
	return hold
}

func pruneMustBeBusy(t *testing.T, dest string) {
	t.Helper()
	hold, ok, err := publish.TryLock(dest)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		_ = hold.Release()
		t.Fatalf("TryLock(%s) succeeded while an overlapping destination was held", dest)
	}
}

func TestPrune_NonOverlappingReplacesRunConcurrently(t *testing.T) {
	store := t.TempDir()
	a := pruneMustHold(t, filepath.Join(store, "a"))
	defer func() { _ = a.Release() }()
	sibling := pruneMustHold(t, filepath.Join(store, "b"))
	defer func() { _ = sibling.Release() }()
	unrelated := pruneMustHold(t, filepath.Join(t.TempDir(), "c"))
	_ = unrelated.Release()
}

func TestPrune_SameDestinationSerializes(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "pkg")
	hold := pruneMustHold(t, dest)
	pruneMustBeBusy(t, dest)
	if err := hold.Release(); err != nil {
		t.Fatal(err)
	}
	_ = pruneMustHold(t, dest).Release()
}

func TestPrune_ParentAndChildDestinationsNeverOverlap(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "node_modules")
	child := filepath.Join(parent, "pkg", "node_modules", "dep")
	if err := os.MkdirAll(filepath.Dir(child), 0o755); err != nil {
		t.Fatal(err)
	}
	hold := pruneMustHold(t, parent)
	pruneMustBeBusy(t, child)
	_ = hold.Release()
	hold = pruneMustHold(t, child)
	pruneMustBeBusy(t, parent)
	_ = hold.Release()
}

// Process roles for re-executing this test binary.
const (
	pruneRoleEnv     = "EVO_PRUNE_TEST_ROLE"
	pruneDestEnv     = "EVO_PRUNE_TEST_DEST"
	pruneArgEnv      = "EVO_PRUNE_TEST_ARG"
	pruneRoleReplace = "replace"
	pruneRoleCrash   = "crash"
	// pruneExitChanged is the replacer's exit code for ErrTreeChanged.
	pruneExitChanged = 3
)

// TestPrune_ProcessHelper is not a test: it is the body of a re-executed
// child process, selected by pruneRoleEnv.
func TestPrune_ProcessHelper(t *testing.T) {
	switch os.Getenv(pruneRoleEnv) {
	case pruneRoleReplace:
		os.Exit(pruneReplaceChild(t))
	case pruneRoleCrash:
		pruneCrashChild(t)
	default:
		t.Skip("helper process only")
	}
}

// pruneReplaceChild waits at the start barrier (its stdin reaching EOF), then replaces the expected
// tree with the archive it was handed.
func pruneReplaceChild(t *testing.T) int {
	_, _ = io.Copy(io.Discard, os.Stdin)
	dest := os.Getenv(pruneDestEnv)
	expected, archivePath, _ := splitPair(os.Getenv(pruneArgEnv))
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		return evo.Tree{Path: dest, Content: evo.Extract{File: evo.File{Path: archivePath}}}.Replace(ctx, expected)
	})
	switch {
	case err == nil:
		return 0
	case errors.Is(err, evo.ErrTreeChanged):
		return pruneExitChanged
	default:
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
}

func splitPair(s string) (string, string, bool) {
	for i := range len(s) {
		if s[i] == '\n' {
			return s[:i], s[i+1:], true
		}
	}
	return s, "", false
}

func pruneChild(t *testing.T, env ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestPrune_ProcessHelper$", "-test.count=1")
	cmd.Env = append(os.Environ(), env...)
	cmd.Stderr = os.Stderr
	return cmd
}

func TestPrune_TwoProcessesReplacingOneDestinationLeaveExactlyOneValidResult(t *testing.T) {
	f := newPruneReplaceFixture(t)
	contents := []map[string]string{{"index.js": "from A"}, {"index.js": "from B"}}
	children := make([]*exec.Cmd, len(contents))
	starts := make([]io.Closer, len(contents))
	for i, files := range contents {
		archivePath := archive(t, t.TempDir(), "p.tgz", files).Path
		children[i] = pruneChild(t, pruneRoleEnv+"="+pruneRoleReplace, pruneDestEnv+"="+f.dest,
			pruneArgEnv+"="+f.expected+"\n"+archivePath)
		start, err := children[i].StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		starts[i] = start
		if err := children[i].Start(); err != nil {
			t.Fatal(err)
		}
	}
	// The start barrier: every child blocks reading stdin until EOF, so
	// closing the pipes releases both only once both are running.
	for _, start := range starts {
		_ = start.Close()
	}
	var won, refused int
	for i, cmd := range children {
		err := cmd.Wait()
		var exitErr *exec.ExitError
		switch {
		case err == nil:
			won++
		case errors.As(err, &exitErr) && exitErr.ExitCode() == pruneExitChanged:
			refused++
		default:
			t.Fatalf("replacer %d: %v", i, err)
		}
	}
	if won != 1 || refused != 1 {
		t.Fatalf("won=%d refused=%d, want exactly one commit and one ErrTreeChanged", won, refused)
	}
	got := onDisk(t, f.dest)
	if !equalFiles(got, contents[0]) && !equalFiles(got, contents[1]) {
		t.Fatalf("destination = %v, want exactly one replacer's whole tree", got)
	}
	if names := advNames(t, f.parent); !slices.Equal(names, []string{"pkg"}) {
		t.Fatalf("concurrent replacers left %v in the parent", names)
	}
}
