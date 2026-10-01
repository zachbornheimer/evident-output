package find_test

// Adversarial hardening tests for evo.Find. Each test is named for the
// weakness it guards; sources are in docs/zys-1382/adversarial-research.md.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

const advLiveness = 10 * time.Second

// Bounds the contract asserts for "bounded-parallel". The ticket gives no
// number; these are generous ceilings that an unbounded walker (one
// goroutine or one open directory per entry) blows through on the wide
// trees below, and that any pool sized to the machine stays under.
const (
	advMaxExtraGoroutines = 256 // peak goroutines above the pre-search baseline
	advMaxOpenDirs        = 128 // open descriptors above the pre-search baseline
)

func advRun(tb testing.TB, cfg evo.Config, fn func(context.Context) error) error {
	tb.Helper()
	cfg.Isolated, cfg.Plain = true, true
	cfg.Stdout, cfg.Stderr = io.Discard, io.Discard
	if cfg.StateDir == "" {
		cfg.StateDir = tb.TempDir()
	}
	out := evo.Init(cfg)
	defer func() { _ = out.Close() }()
	err := out.Task("adversarial").Define(fn).Wait()
	_ = out.Finish()
	return err
}

func advTouch(tb testing.TB, root string, rels ...string) {
	tb.Helper()
	for _, rel := range rels {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			tb.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
			tb.Fatal(err)
		}
	}
}

// advFind runs evo.Find and returns matched paths relative to root, in the
// order Find returned them. It fails the test when Find outlives
// advLiveness.
func advFind(t *testing.T, root string, names ...string) ([]string, []evo.File, error) {
	t.Helper()
	type result struct {
		files []evo.File
		err   error
	}
	done := make(chan result, 1)
	go func() {
		var files []evo.File
		err := advRun(t, evo.Config{}, func(ctx context.Context) error {
			var err error
			files, err = evo.Find(ctx, root, names...)
			return err
		})
		done <- result{files, err}
	}()
	select {
	case r := <-done:
		rels := make([]string, len(r.files))
		for i, f := range r.files {
			rel, err := filepath.Rel(root, f.Path)
			if err != nil {
				rel = f.Path
			}
			rels[i] = filepath.ToSlash(rel)
		}
		return rels, r.files, r.err
	case <-time.After(advLiveness):
		t.Fatalf("Find did not return within %v", advLiveness)
		return nil, nil, nil
	}
}

func advWideTree(tb testing.TB, root string, dirs, filesPerDir int) {
	tb.Helper()
	for d := range dirs {
		dir := filepath.Join(root, fmt.Sprintf("pkg%04d", d))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			tb.Fatal(err)
		}
		for f := range filesPerDir {
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%02d.js", f)), nil, 0o644); err != nil {
				tb.Fatal(err)
			}
		}
		if d%10 == 0 {
			advTouch(tb, dir, "package.json")
		}
	}
}

func TestAdversarial_SymlinkLoopTerminates(t *testing.T) {
	root := t.TempDir()
	advTouch(t, root, "a/package.json")
	if err := os.Symlink("..", filepath.Join(root, "a", "up")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("loop", filepath.Join(root, "loop")); err != nil {
		t.Fatal(err)
	}
	got, _, err := advFind(t, root, "package.json")
	if err != nil {
		t.Fatalf("Find over symlink loops: %v", err)
	}
	if !slices.Equal(got, []string{"a/package.json"}) {
		t.Fatalf("Find = %v, want exactly [a/package.json]", got)
	}
}

func TestAdversarial_SymlinkedDirectoryOutsideRootNotFollowed(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	advTouch(t, outside, "package.json")
	advTouch(t, root, "inside/package.json")
	if err := os.Symlink(outside, filepath.Join(root, "ext")); err != nil {
		t.Fatal(err)
	}
	got, _, err := advFind(t, root, "package.json")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"inside/package.json"}) {
		t.Fatalf("Find followed a symlink out of the root: %v", got)
	}
}

func TestAdversarial_ExactBasenameNotSubstringOrGlob(t *testing.T) {
	root := t.TempDir()
	advTouch(t, root, "package.json", "package.json.bak", "my-package.json", "packageXjson", "Package.JSON.orig", "d/package.json~")
	got, _, err := advFind(t, root, "package.json")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"package.json"}) {
		t.Fatalf("Find matched by substring or pattern: %v", got)
	}
}

// Structural: Find performs zero content reads, so mode-000 matches are
// still found.
func TestAdversarial_UnreadableMatchesStillFound(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads mode-000 files")
	}
	root := t.TempDir()
	advTouch(t, root, "a/package.json", "b/pnpm-lock.yaml")
	for _, rel := range []string{"a/package.json", "b/pnpm-lock.yaml"} {
		path := filepath.Join(root, rel)
		if err := os.Chmod(path, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
	}
	got, _, err := advFind(t, root, "package.json", "pnpm-lock.yaml")
	if err != nil {
		t.Fatalf("Find opened a match: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Find = %v, want both unreadable matches", got)
	}
}

func TestAdversarial_ResultsCarryNoContent(t *testing.T) {
	root := t.TempDir()
	advTouch(t, root, "package.json", "x/package.json")
	_, files, err := advFind(t, root, "package.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f.Content != nil {
			t.Fatalf("Find loaded content for %s; results must be content-lazy", f.Path)
		}
	}
}

func TestAdversarial_FIFONamedLikeTargetDoesNotBlock(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "p"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(root, "p", "package.json"), 0o644); err != nil {
		t.Skipf("mkfifo unsupported: %v", err)
	}
	advTouch(t, root, "q/package.json")
	_, _, _ = advFind(t, root, "package.json") // advFind enforces liveness
}

func TestAdversarial_CanceledContextStopsWalk(t *testing.T) {
	root := t.TempDir()
	advWideTree(t, root, 500, 4)
	start := time.Now()
	err := advRun(t, evo.Config{}, func(ctx context.Context) error {
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		_, err := evo.Find(cctx, root, "package.json")
		return err
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Find with a canceled context = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(start); elapsed > advLiveness {
		t.Fatalf("canceled Find took %v", elapsed)
	}
}

func TestAdversarial_UnreadableDirectoryDoesNotHang(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root lists mode-000 directories")
	}
	root := t.TempDir()
	advTouch(t, root, "ok/package.json", "locked/inner/package.json")
	locked := filepath.Join(root, "locked")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	got, _, err := advFind(t, root, "package.json")
	if err != nil {
		// Reporting the denied directory is acceptable; hiding it behind a
		// generic error is not.
		if !errors.Is(err, fs.ErrPermission) || !strings.Contains(err.Error(), "locked") {
			t.Fatalf("Find error = %v, want fs.ErrPermission naming the locked directory", err)
		}
		return
	}
	if !slices.Equal(got, []string{"ok/package.json"}) {
		t.Fatalf("Find skipped the locked directory but returned %v, want [ok/package.json]", got)
	}
}

func TestAdversarial_ResultOrderIsDeterministic(t *testing.T) {
	root := t.TempDir()
	advWideTree(t, root, 200, 2)
	first, _, err := advFind(t, root, "package.json")
	if err != nil {
		t.Fatal(err)
	}
	for range 5 {
		again, _, err := advFind(t, root, "package.json")
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(first, again) {
			t.Fatal("Find returned the same matches in a different order")
		}
	}
	if !slices.IsSorted(first) {
		t.Fatalf("Find results are not sorted by path: %v", first[:min(5, len(first))])
	}
}

func TestAdversarial_ConcurrentIdenticalSearchesAgree(t *testing.T) {
	const searches = 8
	root := t.TempDir()
	advWideTree(t, root, 300, 2)
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: t.TempDir()})
	defer func() { _ = out.Close() }()
	group := out.Group("searches")
	results := make([][]evo.File, searches)
	for i := range searches {
		group.Task(fmt.Sprintf("s%d", i)).Define(func(ctx context.Context) error {
			var err error
			results[i], err = evo.Find(ctx, root, "package.json")
			return err
		})
	}
	if err := group.Wait(); err != nil {
		t.Fatal(err)
	}
	_ = out.Finish()
	paths := func(files []evo.File) []string {
		got := make([]string, len(files))
		for i, f := range files {
			got[i] = f.Path
		}
		return got
	}
	for i := 1; i < searches; i++ {
		if !slices.Equal(paths(results[0]), paths(results[i])) {
			t.Fatalf("concurrent identical searches disagree (search 0: %d, search %d: %d)", len(results[0]), i, len(results[i]))
		}
	}
	if len(results[0]) != 30 {
		t.Fatalf("Find found %d matches, want 30", len(results[0]))
	}
}

func TestAdversarial_MissingRootIsError(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	if _, _, err := advFind(t, root, "package.json"); err == nil {
		t.Fatal("Find under a missing root reported success (indistinguishable from no matches)")
	}
}

func BenchmarkFind_WideTree(b *testing.B) {
	root := b.TempDir()
	advWideTree(b, root, 5000, 4) // 5000 dirs, 20k files, 500 matches
	err := advRun(b, evo.Config{}, func(ctx context.Context) error {
		for b.Loop() {
			files, err := evo.Find(ctx, root, "package.json", "pnpm-lock.yaml")
			if err != nil {
				return err
			}
			if len(files) != 500 {
				return fmt.Errorf("found %d, want 500", len(files))
			}
		}
		return nil
	})
	if err != nil {
		b.Fatal(err)
	}
}

func advOpenDescriptors(tb testing.TB) int {
	tb.Helper()
	entries, err := os.ReadDir("/dev/fd")
	if err != nil {
		tb.Skipf("cannot count descriptors: %v", err)
	}
	return len(entries)
}

// limitDescriptors lowers the soft RLIMIT_NOFILE to headroom above what the
// process holds now, so a walker that opens directories without a bound
// fails with EMFILE instead of passing on a generous default.
func limitDescriptors(t *testing.T, headroom int) {
	t.Helper()
	var old syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &old); err != nil {
		t.Skipf("getrlimit: %v", err)
	}
	limit := old
	limit.Cur = uint64(advOpenDescriptors(t) + headroom)
	if limit.Cur >= old.Cur {
		t.Skipf("soft limit %d already below the test ceiling", old.Cur)
	}
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &limit); err != nil {
		t.Skipf("setrlimit: %v", err)
	}
	t.Cleanup(func() { _ = syscall.Setrlimit(syscall.RLIMIT_NOFILE, &old) })
}

// Bounded parallelism, descriptor side: 3000 sibling directories must be
// searched with at most advMaxOpenDirs descriptors above the baseline.
func TestAdversarial_ParallelismIsBoundedByDescriptors(t *testing.T) {
	root := t.TempDir()
	advWideTree(t, root, 3000, 1)
	limitDescriptors(t, advMaxOpenDirs)
	got, _, err := advFind(t, root, "package.json")
	if err != nil {
		t.Fatalf("Find exhausted descriptors on a wide tree: %v", err)
	}
	if len(got) != 300 {
		t.Fatalf("Find found %d matches, want 300", len(got))
	}
}

// Bounded parallelism, goroutine side: sampled peak goroutines stay within
// advMaxExtraGoroutines while 4000 sibling directories are walked.
func TestAdversarial_ParallelismIsBoundedByGoroutines(t *testing.T) {
	root := t.TempDir()
	advWideTree(t, root, 4000, 1)
	baseline := runtime.NumGoroutine()
	var peak atomic.Int64
	stop, sampled := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(sampled)
		for {
			select {
			case <-stop:
				return
			default:
				if n := int64(runtime.NumGoroutine()); n > peak.Load() {
					peak.Store(n)
				}
				runtime.Gosched()
			}
		}
	}()
	got, _, err := advFind(t, root, "package.json")
	close(stop)
	<-sampled
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 400 {
		t.Fatalf("Find found %d matches, want 400", len(got))
	}
	if extra := int(peak.Load()) - baseline; extra > advMaxExtraGoroutines {
		t.Fatalf("Find peaked at %d goroutines above baseline, want <= %d (unbounded fan-out)", extra, advMaxExtraGoroutines)
	}
}

// A search leaves no goroutine behind, on success or on cancellation.
func TestAdversarial_NoGoroutineLeakAfterSearch(t *testing.T) {
	root := t.TempDir()
	advWideTree(t, root, 400, 2)
	_, _, _ = advFind(t, root, "package.json") // warm up runtime-owned goroutines
	baseline := runtime.NumGoroutine()
	for range 5 {
		_, _, _ = advFind(t, root, "package.json")
		_ = advRun(t, evo.Config{}, func(ctx context.Context) error {
			cctx, cancel := context.WithCancel(ctx)
			cancel()
			_, err := evo.Find(cctx, root, "package.json")
			return err
		})
	}
	deadline := time.Now().Add(advLiveness)
	for runtime.NumGoroutine() > baseline+2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if extra := runtime.NumGoroutine() - baseline; extra > 2 {
		t.Fatalf("%d goroutines outlived their searches", extra)
	}
}

// A 200-deep chain is walked without holding a descriptor per level and
// without recursion that scales with depth.
func TestAdversarial_DeepChainIsFoundWithBoundedDescriptors(t *testing.T) {
	root := t.TempDir()
	dir := root
	for range 200 {
		dir = filepath.Join(dir, "d")
	}
	advTouch(t, dir, "package.json")
	limitDescriptors(t, 64)
	got, _, err := advFind(t, root, "package.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || strings.Count(got[0], "/") != 200 {
		t.Fatalf("Find = %v, want the single match 200 levels down", got)
	}
}

// Cancelling mid-walk either stops with context.Canceled or has already
// finished with the complete answer. A partial answer with a nil error is
// the failure.
func TestAdversarial_CancelMidWalkNeverReturnsPartialSuccess(t *testing.T) {
	root := t.TempDir()
	advWideTree(t, root, 3000, 2) // 300 matches
	for range 10 {
		var files []evo.File
		err := advRun(t, evo.Config{}, func(ctx context.Context) error {
			cctx, cancel := context.WithCancel(ctx)
			go func() { time.Sleep(time.Millisecond); cancel() }()
			defer cancel()
			var err error
			files, err = evo.Find(cctx, root, "package.json")
			return err
		})
		switch {
		case errors.Is(err, context.Canceled):
		case err == nil && len(files) == 300:
		default:
			t.Fatalf("mid-walk cancel gave err=%v with %d files, want Canceled or the full 300", err, len(files))
		}
	}
}

// A deadline that expires mid-walk is reported as DeadlineExceeded within
// the liveness window, never as a silent short result.
func TestAdversarial_DeadlineMidWalkIsReported(t *testing.T) {
	root := t.TempDir()
	advWideTree(t, root, 3000, 2)
	var files []evo.File
	start := time.Now()
	err := advRun(t, evo.Config{}, func(ctx context.Context) error {
		cctx, cancel := context.WithTimeout(ctx, time.Millisecond)
		defer cancel()
		var err error
		files, err = evo.Find(cctx, root, "package.json")
		return err
	})
	if time.Since(start) > advLiveness {
		t.Fatalf("deadline Find took %v", time.Since(start))
	}
	if !errors.Is(err, context.DeadlineExceeded) && (err != nil || len(files) != 300) {
		t.Fatalf("mid-walk deadline gave err=%v with %d files, want DeadlineExceeded or the full 300", err, len(files))
	}
}

// Coalescing must not couple callers' lifetimes: one cancelled searcher
// sharing a traversal cannot cancel the others.
//
// Settled ZYS-1382 dispute (test was wrong): the barrier needs peers+1 Tasks
// running at once, and the SPEC promises no concurrency beyond
// Config.MaxConcurrency (default GOMAXPROCS). The Config therefore sizes
// MaxConcurrency to the barrier instead of depending on the host's CPU count.
func TestAdversarial_CancelledSearcherDoesNotCancelCoalescedPeers(t *testing.T) {
	root := t.TempDir()
	advWideTree(t, root, 2000, 2) // 200 matches
	const peers = 4
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: t.TempDir(), MaxConcurrency: peers + 1})
	defer func() { _ = out.Close() }()
	group := out.Group("peers")
	var (
		mu      sync.Mutex
		counts  []int
		started sync.WaitGroup
		release = make(chan struct{})
	)
	started.Add(peers + 1)
	var cancelledErr error
	group.Task("cancelled").Define(func(ctx context.Context) error {
		cctx, cancel := context.WithCancel(ctx)
		started.Done()
		<-release
		go func() { time.Sleep(time.Millisecond); cancel() }()
		_, cancelledErr = evo.Find(cctx, root, "package.json")
		return nil
	})
	for i := range peers {
		group.Task(fmt.Sprintf("peer%d", i)).Define(func(ctx context.Context) error {
			started.Done()
			<-release
			files, err := evo.Find(ctx, root, "package.json")
			if err != nil {
				return err
			}
			mu.Lock()
			counts = append(counts, len(files))
			mu.Unlock()
			return nil
		})
	}
	go func() { started.Wait(); close(release) }()
	done := make(chan error, 1)
	go func() { done <- group.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("a peer failed because another searcher was cancelled: %v", err)
		}
	case <-time.After(advLiveness * 3):
		t.Fatal("coalesced searches did not finish")
	}
	_ = out.Finish()
	if cancelledErr != nil && !errors.Is(cancelledErr, context.Canceled) {
		t.Fatalf("cancelled searcher = %v, want nil or context.Canceled", cancelledErr)
	}
	if len(counts) != peers {
		t.Fatalf("%d of %d peers reported", len(counts), peers)
	}
	for _, n := range counts {
		if n != 200 {
			t.Fatalf("a peer got %d matches, want 200", n)
		}
	}
}

// Finds with different names over one root are different searches and must
// not share results.
func TestAdversarial_DifferentNamesNeverShareResults(t *testing.T) {
	root := t.TempDir()
	advTouch(t, root, "a/package.json", "a/pnpm-lock.yaml", "b/package.json")
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: t.TempDir()})
	defer func() { _ = out.Close() }()
	group := out.Group("distinct")
	var pkg, lock []evo.File
	group.Task("pkg").Define(func(ctx context.Context) (err error) {
		pkg, err = evo.Find(ctx, root, "package.json")
		return err
	})
	group.Task("lock").Define(func(ctx context.Context) (err error) {
		lock, err = evo.Find(ctx, root, "pnpm-lock.yaml")
		return err
	})
	if err := group.Wait(); err != nil {
		t.Fatal(err)
	}
	_ = out.Finish()
	if len(pkg) != 2 || len(lock) != 1 {
		t.Fatalf("package.json search found %d, pnpm-lock.yaml search found %d; want 2 and 1", len(pkg), len(lock))
	}
}

// Same names in a different order or repeated are the same search and
// must give the same answer.
func TestAdversarial_NameOrderDoesNotChangeTheAnswer(t *testing.T) {
	root := t.TempDir()
	advTouch(t, root, "a/package.json", "b/pnpm-lock.yaml")
	forward, _, err := advFind(t, root, "package.json", "pnpm-lock.yaml")
	if err != nil {
		t.Fatal(err)
	}
	reverse, _, err := advFind(t, root, "pnpm-lock.yaml", "package.json", "package.json")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(forward, reverse) {
		t.Fatalf("name order changed the answer: %v vs %v", forward, reverse)
	}
}

// Entries created and removed while the walk runs are a normal race, not an
// error: Find must not fail on a directory that vanishes between listing and
// descent, and every file that stays put for the whole search is returned.
func TestAdversarial_ConcurrentChurnDuringWalkIsNotAnError(t *testing.T) {
	root := t.TempDir()
	advWideTree(t, root, 400, 2) // 40 stable matches
	churn := filepath.Join(root, "churn")
	stop, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			dir := filepath.Join(churn, fmt.Sprintf("d%d", i%32))
			_ = os.MkdirAll(dir, 0o755)
			_ = os.WriteFile(filepath.Join(dir, "package.json"), nil, 0o644)
			_ = os.RemoveAll(filepath.Join(churn, fmt.Sprintf("d%d", (i+16)%32)))
		}
	}()
	defer func() { close(stop); <-stopped }()
	for range 20 {
		got, _, err := advFind(t, root, "package.json")
		if err != nil {
			t.Fatalf("Find failed on a tree that changed under it: %v", err)
		}
		stable := 0
		for _, rel := range got {
			if strings.HasPrefix(rel, "pkg") {
				stable++
			}
		}
		if stable != 40 {
			t.Fatalf("Find returned %d of the 40 files that never moved", stable)
		}
		if !slices.IsSorted(got) {
			t.Fatalf("Find results are not sorted under churn")
		}
	}
}

// A deep, wide and symlink-laden tree still yields each file exactly once.
func TestAdversarial_NoDuplicateResultsWithHardlinksAndLoops(t *testing.T) {
	root := t.TempDir()
	advTouch(t, root, "a/package.json")
	if err := os.Link(filepath.Join(root, "a", "package.json"), filepath.Join(root, "linked-package.json")); err != nil {
		t.Skip("hardlinks unavailable:", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(filepath.Join(root, "a", "package.json"), filepath.Join(root, "b", "package.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(".", filepath.Join(root, "b", "self")); err != nil {
		t.Fatal(err)
	}
	got, _, err := advFind(t, root, "package.json")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"a/package.json", "b/package.json"}) {
		t.Fatalf("Find = %v, want each hard link by its own path, once", got)
	}
}
