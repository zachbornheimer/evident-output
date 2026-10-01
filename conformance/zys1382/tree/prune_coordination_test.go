package tree_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/internal/publish"
)

// Gated replacer protocol over a child's stdin and stdout. The child reads
// one line to start. Once its tree is staged it prints gateStaged and
// reads a line; once it holds the destination's coordination it prints
// gateLocked and reads a line. No step waits on time.
const (
	gateStaged = "staged"
	gateLocked = "locked"
	gateGo     = "go\n"
	// gateDeadline only fails a test that would otherwise hang; nothing
	// waits on it to synchronize.
	gateDeadline = time.Minute
)

// pruneGatedChild replaces its destination, logging "enter" and "exit"
// around everything it does under the destination's coordination, and
// pausing after staging and inside that critical section until its parent
// lets it go on.
func pruneGatedChild(t *testing.T) int {
	in := bufio.NewReader(os.Stdin)
	_, _ = in.ReadString('\n')
	dest := os.Getenv(pruneDestEnv)
	expected, archivePath, _ := splitPair(os.Getenv(pruneArgEnv))
	logPath := os.Getenv(pruneLogEnv)
	pause := func(signal string) {
		fmt.Println(signal)
		_, _ = in.ReadString('\n')
	}
	publish.InjectFaults(publish.Faults{At: func(step publish.Step, at string) {
		if at != dest {
			return
		}
		switch step {
		case publish.StepStaged:
			pause(gateStaged)
		case publish.StepLocked:
			appendLine(logPath, "enter "+dest)
			pause(gateLocked)
		case publish.StepReleasing:
			appendLine(logPath, "exit "+dest)
		}
	}})
	return pruneExitCode(contractRun(t, evo.Config{}, func(ctx context.Context) error {
		return evo.Tree{Path: dest, Content: evo.Extract{File: evo.File{Path: archivePath}}}.Replace(ctx, expected)
	}))
}

// appendLine appends one line to path; O_APPEND keeps two processes'
// lines whole.
func appendLine(path, line string) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		panic(err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(line + "\n"); err != nil {
		panic(err)
	}
}

// gatedReplacer is a running gated child.
type gatedReplacer struct {
	dest string
	cmd  *exec.Cmd
	in   io.WriteCloser
}

// gateEvent is a signal one gated child printed.
type gateEvent struct {
	from   *gatedReplacer
	signal string
}

// startGated starts a gated child replacing dest's expected tree with the
// archived files; its signals arrive on events.
func startGated(t *testing.T, logPath, dest, expected string, files map[string]string, events chan<- gateEvent) *gatedReplacer {
	t.Helper()
	archivePath := archive(t, t.TempDir(), "next.tgz", files).Path
	g := &gatedReplacer{dest: dest}
	g.cmd = pruneChild(t, pruneRoleEnv+"="+pruneRoleGated, pruneDestEnv+"="+dest,
		pruneArgEnv+"="+expected+"\n"+archivePath, pruneLogEnv+"="+logPath)
	var err error
	if g.in, err = g.cmd.StdinPipe(); err != nil {
		t.Fatal(err)
	}
	out, err := g.cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := g.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = g.cmd.Process.Kill() })
	go func() {
		lines := bufio.NewScanner(out)
		for lines.Scan() {
			if text := lines.Text(); text == gateStaged || text == gateLocked {
				events <- gateEvent{from: g, signal: text}
			}
		}
	}()
	return g
}

// next lets g past its current gate.
func (g *gatedReplacer) next(t *testing.T) {
	t.Helper()
	if _, err := io.WriteString(g.in, gateGo); err != nil {
		t.Fatalf("signal %s: %v", g.dest, err)
	}
}

// wait returns the child's exit code.
func (g *gatedReplacer) wait(t *testing.T) int {
	t.Helper()
	_ = g.in.Close()
	err := g.cmd.Wait()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &exitErr):
		return exitErr.ExitCode()
	}
	t.Fatalf("replacer %s: %v", g.dest, err)
	return -1
}

// await returns the child that next prints signal.
func await(t *testing.T, events <-chan gateEvent, signal string) *gatedReplacer {
	t.Helper()
	select {
	case e := <-events:
		if e.signal != signal {
			t.Fatalf("%s signalled %q, want %q", e.from.dest, e.signal, signal)
		}
		return e.from
	case <-time.After(gateDeadline):
		t.Fatalf("no replacer signalled %q", signal)
		return nil
	}
}

func readLog(t *testing.T, path string) []string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(body)), "\n")
}

// Two processes replace a parent tree and a tree inside it at once, in
// each order. Both stage first (so the child's stage sits inside the
// parent); the first takes the coordination, the second is released while
// the first is still inside its critical section. They never interleave,
// and the result is serial: the first commits and the second is refused
// with ErrTreeChanged, changing nothing.
func TestPrune_ParentAndChildReplacesAcrossProcessesSerialize(t *testing.T) {
	for _, parentFirst := range []bool{true, false} {
		t.Run(map[bool]string{true: "parent first", false: "child first"}[parentFirst], func(t *testing.T) {
			pruneParentAndChildInOrder(t, parentFirst)
		})
	}
}

func pruneParentAndChildInOrder(t *testing.T, parentFirst bool) {
	parent := filepath.Join(t.TempDir(), "node_modules")
	child := filepath.Join(parent, "pkg", "node_modules", "dep")
	plant(t, parent, map[string]string{"a.js": "parent old", "pkg/node_modules/dep/index.js": "dep old"})
	parentExpected, childExpected := pruneChecksum(t, parent), pruneChecksum(t, child)
	logPath := filepath.Join(t.TempDir(), "critical.log")
	events := make(chan gateEvent, 2)
	p := startGated(t, logPath, parent, parentExpected,
		map[string]string{"a.js": "parent new", "pkg/node_modules/dep/index.js": "dep old"}, events)
	c := startGated(t, logPath, child, childExpected, map[string]string{"index.js": "dep new"}, events)
	for _, g := range []*gatedReplacer{p, c} {
		g.next(t)
	}
	for range 2 {
		await(t, events, gateStaged)
	}
	firstG, secondG := p, c
	if !parentFirst {
		firstG, secondG = c, p
	}
	firstG.next(t)
	if holder := await(t, events, gateLocked); holder != firstG {
		t.Fatalf("%s took the coordination, want %s", holder.dest, firstG.dest)
	}
	// From outside both processes: the holder excludes the parent and the
	// child alike while it is inside its critical section.
	pruneMustBeBusy(t, parent)
	pruneMustBeBusy(t, child)
	secondG.next(t)
	firstG.next(t)
	if g := await(t, events, gateLocked); g != secondG {
		t.Fatalf("%s took the coordination second, want %s", g.dest, secondG.dest)
	}
	secondG.next(t)
	pCode, cCode := p.wait(t), c.wait(t)

	first, second := firstG.dest, secondG.dest
	if got, want := readLog(t, logPath), []string{"enter " + first, "exit " + first, "enter " + second, "exit " + second}; !slices.Equal(got, want) {
		t.Fatalf("critical sections = %v, want %v (no interleaving)", got, want)
	}
	// Whoever commits first changes the other's tree: a parent swap carries
	// the child's stage away; a child commit changes the parent's digest.
	// The second is refused and changes nothing.
	want := map[string]struct {
		pCode, cCode int
		tree         map[string]string
	}{
		parent: {0, pruneExitChanged, map[string]string{"a.js": "parent new", "pkg/node_modules/dep/index.js": "dep old"}},
		child:  {pruneExitChanged, 0, map[string]string{"a.js": "parent old", "pkg/node_modules/dep/index.js": "dep new"}},
	}[first]
	if got := onDisk(t, parent); pCode != want.pCode || cCode != want.cCode || !equalFiles(got, want.tree) {
		t.Fatalf("%s first: exit codes parent=%d child=%d tree=%v; want %d/%d %v", first, pCode, cCode, got, want.pCode, want.cCode, want.tree)
	}
	for _, dest := range []string{parent, child} {
		if leftovers, err := publish.Leftovers(dest); err != nil || len(leftovers) != 0 {
			t.Fatalf("Leftovers(%s) = %v, %v; want none", dest, leftovers, err)
		}
	}
}

// One process holds a sibling's critical section open; another process's
// Replace of the next sibling runs start to finish meanwhile.
func TestPrune_SiblingReplacesAcrossProcessesDoNotBlock(t *testing.T) {
	store := t.TempDir()
	a, b := filepath.Join(store, "a"), filepath.Join(store, "b")
	plant(t, a, map[string]string{"index.js": "a old"})
	plant(t, b, map[string]string{"index.js": "b old"})
	logPath := filepath.Join(t.TempDir(), "critical.log")
	events := make(chan gateEvent, 2)
	holdA := startGated(t, logPath, a, pruneChecksum(t, a), map[string]string{"index.js": "a new"}, events)
	holdA.next(t)
	await(t, events, gateStaged).next(t)
	await(t, events, gateLocked)

	replaceB := startGated(t, logPath, b, pruneChecksum(t, b), map[string]string{"index.js": "b new"}, events)
	replaceB.next(t)
	await(t, events, gateStaged).next(t)
	await(t, events, gateLocked).next(t)
	done := make(chan int, 1)
	go func() { done <- replaceB.wait(t) }()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("sibling replace exited %d", code)
		}
	case <-time.After(gateDeadline):
		t.Fatal("a sibling's Replace waited on another sibling's critical section")
	}
	if got := onDisk(t, b); got["index.js"] != "b new" {
		t.Fatalf("sibling = %v, want its new tree", got)
	}

	holdA.next(t)
	if code := holdA.wait(t); code != 0 {
		t.Fatalf("held replace exited %d", code)
	}
	if got := onDisk(t, a); got["index.js"] != "a new" {
		t.Fatalf("held sibling = %v, want its new tree", got)
	}
}
