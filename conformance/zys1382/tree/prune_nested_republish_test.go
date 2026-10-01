package tree_test

import (
	"context"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/internal/publish"
)

// nestedRepublishRounds is how many times each concurrent nested
// republish runs, so an interleaving that only some schedules reach shows.
const nestedRepublishRounds = 20

// nestedStoreFiles is a parent package tree with a nested dependency, as a
// content store holds it.
var nestedStoreFiles = map[string]string{
	"package.json":                       "{}",
	"lib/a.js":                           "a",
	"node_modules/dep/package.json":      `{"name":"dep"}`,
	"node_modules/dep/lib/deep/index.js": "dep",
}

// nestedRepublishFixture is a project's private copy of a parent tree and
// of the dependency nested inside it, plus the store trees zq prune
// republishes each from as writable clones.
type nestedRepublishFixture struct {
	parentStore, childStore string
	parent, child           string
	parentExpected          string
	childExpected           string
}

func newNestedRepublishFixture(t *testing.T) nestedRepublishFixture {
	t.Helper()
	if !publish.StagesApart(t.TempDir()) {
		t.Skip("the staging root is on another volume than the test tree: stages fall back to beside their destination")
	}
	parentStore := filepath.Join(t.TempDir(), "parent-store")
	plant(t, parentStore, nestedStoreFiles)
	parent := filepath.Join(t.TempDir(), "pkg")
	pruneByteCopy(t, parentStore, parent)
	f := nestedRepublishFixture{
		parentStore: parentStore,
		childStore:  filepath.Join(parentStore, "node_modules", "dep"),
		parent:      parent,
		child:       filepath.Join(parent, "node_modules", "dep"),
	}
	f.parentExpected, f.childExpected = pruneChecksum(t, f.parent), pruneChecksum(t, f.child)
	return f
}

// republish is zq prune's swap of dest for a writable clone of from.
func republish(ctx context.Context, dest, from, expected string) (evo.ReplaceResult, error) {
	tree := evo.Tree{Path: dest, Content: evo.Clone{From: evo.Tree{Path: from}, Writable: true}}
	return tree.ReplaceTree(ctx, expected, evo.Republish())
}

func (f nestedRepublishFixture) republishParent(t *testing.T) error {
	return f.republishOne(t, f.parent, f.parentStore, f.parentExpected)
}

func (f nestedRepublishFixture) republishChild(t *testing.T) error {
	return f.republishOne(t, f.child, f.childStore, f.childExpected)
}

func (f nestedRepublishFixture) republishOne(t *testing.T, dest, from, expected string) error {
	t.Helper()
	return contractRun(t, evo.Config{}, func(ctx context.Context) error {
		got, err := republish(ctx, dest, from, expected)
		if err == nil && !got.Published {
			t.Errorf("Republish of %s did not publish", dest)
		}
		return err
	})
}

// requireSerialOutcome proves the project holds what republishing the
// parent and the child one after the other (in either order) leaves: the
// store's content, and no staging entry anywhere in it.
func (f nestedRepublishFixture) requireSerialOutcome(t *testing.T) {
	t.Helper()
	if got := onDisk(t, f.parent); !equalFiles(got, nestedStoreFiles) {
		t.Fatalf("project tree = %v, want the store's %v", got, nestedStoreFiles)
	}
	if sum := pruneChecksum(t, f.parent); sum != f.parentExpected {
		t.Fatalf("project tree digests to %s, want %s", sum, f.parentExpected)
	}
	err := filepath.WalkDir(filepath.Dir(f.parent), func(path string, _ fs.DirEntry, err error) error {
		if err == nil && publish.IsStaging(filepath.Base(path)) {
			t.Errorf("staging entry %s left behind", path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The parent's republish commits after the child's writable clone is
// staged and before the child commits. The child still holds what it
// expected, so it commits into the new parent: the outcome is the serial
// order parent, then child.
func TestPrune_RepublishOfAChildAfterItsParentSwappedMidStageSucceeds(t *testing.T) {
	f := newNestedRepublishFixture(t)
	var parentErr error
	ran := false
	defer publish.InjectFaults(publish.Faults{At: func(step publish.Step, at string) {
		if step != publish.StepStaged || at != f.child || ran {
			return
		}
		ran = true
		done := make(chan struct{})
		go func() {
			defer close(done)
			parentErr = f.republishParent(t)
		}()
		<-done
	}})()
	if err := f.republishChild(t); err != nil {
		t.Fatalf("child Republish after its parent's swap = %v, want success", err)
	}
	if !ran || parentErr != nil {
		t.Fatalf("parent Republish ran=%v err=%v, want it to run and succeed", ran, parentErr)
	}
	f.requireSerialOutcome(t)
}

// A parent and a nested child republished at once, released together,
// both succeed every time and leave a serial outcome.
func TestPrune_ConcurrentParentAndChildRepublishInProcessBothSucceed(t *testing.T) {
	for range nestedRepublishRounds {
		f := newNestedRepublishFixture(t)
		start := make(chan struct{})
		errs := make([]error, 2)
		var wg sync.WaitGroup
		for i, run := range []func(*testing.T) error{f.republishParent, f.republishChild} {
			wg.Go(func() {
				<-start
				errs[i] = run(t)
			})
		}
		close(start)
		wg.Wait()
		if errs[0] != nil || errs[1] != nil {
			t.Fatalf("concurrent Republish: parent = %v, child = %v; want both to succeed", errs[0], errs[1])
		}
		f.requireSerialOutcome(t)
	}
}

// pruneRepublishChild is the body of a re-executed process: it waits at
// the start barrier (its stdin reaching EOF), then republishes its
// destination as a writable clone of the tree it was handed.
func pruneRepublishChild(t *testing.T) int {
	_, _ = io.Copy(io.Discard, os.Stdin)
	expected, from, _ := splitPair(os.Getenv(pruneArgEnv))
	dest := os.Getenv(pruneDestEnv)
	return pruneExitCode(contractRun(t, evo.Config{}, func(ctx context.Context) error {
		_, err := republish(ctx, dest, from, expected)
		return err
	}))
}

// Two processes republish a parent and a nested child, released together
// through a pipe barrier: both succeed every time and leave a serial
// outcome.
func TestPrune_ConcurrentParentAndChildRepublishAcrossProcessesBothSucceed(t *testing.T) {
	for range nestedRepublishRounds {
		f := newNestedRepublishFixture(t)
		jobs := []struct{ dest, from, expected string }{
			{f.parent, f.parentStore, f.parentExpected},
			{f.child, f.childStore, f.childExpected},
		}
		procs := make([]*exec.Cmd, len(jobs))
		starts := make([]io.Closer, len(jobs))
		for i, job := range jobs {
			procs[i] = pruneChild(t, pruneRoleEnv+"="+pruneRoleRepublish, pruneDestEnv+"="+job.dest,
				pruneArgEnv+"="+job.expected+"\n"+job.from)
			start, err := procs[i].StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			starts[i] = start
			if err := procs[i].Start(); err != nil {
				t.Fatal(err)
			}
		}
		for _, start := range starts {
			_ = start.Close()
		}
		for i, proc := range procs {
			if err := proc.Wait(); err != nil {
				t.Fatalf("Republish of %s in its own process: %v; want success", jobs[i].dest, err)
			}
		}
		f.requireSerialOutcome(t)
	}
}
