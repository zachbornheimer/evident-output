package file_test

// Adversarial hardening tests for evo.File. Each test is named for the
// weakness it guards; sources are in docs/zys-1382/adversarial-research.md.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

const (
	advLiveness     = 10 * time.Second
	advHelperEnv    = "EVO_ADV_FILE_HELPER"
	advHelperPath   = "EVO_ADV_FILE_PATH"
	advHelperLetter = "EVO_ADV_FILE_LETTER"
	advHelperRounds = "EVO_ADV_FILE_ROUNDS"
	advHelperSize   = 256 << 10
)

// advRun runs fn inside one Task's Define callback on an isolated Output.
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

func advSeed(tb testing.TB, path, body string) {
	tb.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		tb.Fatal(err)
	}
}

func advRead(tb testing.TB, path string) string {
	tb.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		tb.Fatal(err)
	}
	return string(got)
}

// A reader racing a rewrite must see the whole old or whole new content,
// never a torn, empty, or missing file (temp + rename, not in-place write).
func TestAdversarial_TornWriteNeverVisibleToConcurrentReader(t *testing.T) {
	const size = 1 << 20
	path := filepath.Join(t.TempDir(), "blob")
	a, b := bytes.Repeat([]byte{'a'}, size), bytes.Repeat([]byte{'b'}, size)
	advSeed(t, path, string(a))

	var done atomic.Bool
	var torn atomic.Value
	var wg sync.WaitGroup
	wg.Go(func() {
		for !done.Load() {
			got, err := os.ReadFile(path)
			if err != nil {
				torn.Store(fmt.Sprintf("read failed mid-write: %v", err))
				return
			}
			if len(got) != size || (!bytes.Equal(got, a) && !bytes.Equal(got, b)) {
				torn.Store(fmt.Sprintf("observed %d bytes of mixed content", len(got)))
				return
			}
		}
	})
	err := advRun(t, evo.Config{}, func(ctx context.Context) error {
		for i := range 40 {
			body := a
			if i%2 == 0 {
				body = b
			}
			if err := (evo.File{Path: path, Content: evo.Bytes(body)}).Write(ctx); err != nil {
				return err
			}
		}
		return nil
	})
	done.Store(true)
	wg.Wait()
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if msg := torn.Load(); msg != nil {
		t.Fatal(msg)
	}
}

// pnpm's store hardlinks into node_modules: an in-place write through one
// link corrupts every other link to the inode.
func TestAdversarial_WriteDoesNotMutateHardlinkedInode(t *testing.T) {
	dir := t.TempDir()
	store, linked := filepath.Join(dir, "store-blob"), filepath.Join(dir, "linked")
	advSeed(t, store, "store bytes")
	if err := os.Link(store, linked); err != nil {
		t.Skipf("hardlinks unsupported: %v", err)
	}
	err := advRun(t, evo.Config{}, func(ctx context.Context) error {
		return evo.File{Path: linked, Content: evo.Bytes("new bytes")}.Write(ctx)
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := advRead(t, store); got != "store bytes" {
		t.Fatalf("Write through a hardlink changed the other link to %q", got)
	}
	if got := advRead(t, linked); got != "new bytes" {
		t.Fatalf("destination = %q, want new bytes", got)
	}
}

// A planted destination symlink must never redirect the write.
func TestAdversarial_WriteDoesNotFollowDestinationSymlink(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret")
	advSeed(t, outside, "secret")
	link := filepath.Join(dir, "config")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	_ = advRun(t, evo.Config{}, func(ctx context.Context) error {
		return evo.File{Path: link, Content: evo.Bytes("attacker")}.Write(ctx)
	})
	if got := advRead(t, outside); got != "secret" {
		t.Fatalf("Write followed the destination symlink and wrote %q outside", got)
	}
}

func TestAdversarial_WriteRefusesDirectoryAtPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "occupied")
	child := filepath.Join(path, "keep.txt")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	advSeed(t, child, "keep")
	err := advRun(t, evo.Config{}, func(ctx context.Context) error {
		return evo.File{Path: path, Content: evo.Bytes("x")}.Write(ctx)
	})
	if err == nil {
		t.Fatal("Write over a directory succeeded")
	}
	if got := advRead(t, child); got != "keep" {
		t.Fatalf("directory content changed to %q", got)
	}
}

// Temp files start at 0600 and umask filters creation modes; the published
// file must still carry exactly the declared Mode.
func TestAdversarial_ModeIsExactRegardlessOfUmask(t *testing.T) {
	old := syscall.Umask(0o077)
	defer syscall.Umask(old)
	for _, mode := range []fs.FileMode{0o644, 0o755, 0o640} {
		path := filepath.Join(t.TempDir(), "f")
		err := advRun(t, evo.Config{}, func(ctx context.Context) error {
			return evo.File{Path: path, Content: evo.Bytes("x"), Mode: mode}.Write(ctx)
		})
		if err != nil {
			t.Fatalf("Write mode %v: %v", mode, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != mode {
			t.Fatalf("mode = %v under umask 077, want %v", info.Mode().Perm(), mode)
		}
	}
}

func TestAdversarial_CanceledWritePublishesNothingAndLeaksNoTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f")
	err := advRun(t, evo.Config{}, func(ctx context.Context) error {
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		return evo.File{Path: path, Content: evo.Bytes("x")}.Write(cctx)
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Write = %v, want context.Canceled", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("canceled Write left %d entries (first %q)", len(entries), entries[0].Name())
	}
}

func TestAdversarial_ConcurrentInProcessWritersSerialize(t *testing.T) {
	const writers = 16
	path := filepath.Join(t.TempDir(), "shared")
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: t.TempDir()})
	defer func() { _ = out.Close() }()
	group := out.Group("writers")
	for i := range writers {
		body := strings.Repeat(string(rune('a'+i)), advHelperSize)
		group.Task(fmt.Sprintf("w%d", i)).Define(func(ctx context.Context) error {
			return evo.File{Path: path, Content: evo.Bytes(body)}.Write(ctx)
		})
	}
	if err := group.Wait(); err != nil {
		t.Fatalf("writers: %v", err)
	}
	_ = out.Finish()
	advRequireUniform(t, path)
}

// Cross-process writers coordinate through an OS-backed lock: the file
// ends as one writer's whole content, and every Write reports success.
func TestAdversarial_ConcurrentProcessesNeverInterleave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shared")
	var cmds []*exec.Cmd
	for _, letter := range []string{"p", "q", "r", "s"} {
		cmd := advHelper(t, path, letter, "20")
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		cmds = append(cmds, cmd)
	}
	for _, cmd := range cmds {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("helper writer failed: %v", err)
		}
	}
	advRequireUniform(t, path)
}

// A writer that dies mid-run must not leave ownership that blocks the next
// writer or needs caller cleanup.
func TestAdversarial_CrashedWriterLeavesNoStaleLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shared")
	cmd := advHelper(t, path, "z", "0")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(advLiveness)
	for {
		if _, err := os.Stat(path); err == nil || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()

	err := advRun(t, evo.Config{}, func(ctx context.Context) error {
		cctx, cancel := context.WithTimeout(ctx, advLiveness)
		defer cancel()
		return evo.File{Path: path, Content: evo.Bytes("after crash")}.Write(cctx)
	})
	if err != nil {
		t.Fatalf("Write after a crashed writer: %v", err)
	}
	if got := advRead(t, path); got != "after crash" {
		t.Fatalf("content = %q", got)
	}
}

func TestAdversarial_VerifyDetectsOutOfBandEdit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	desired := evo.File{Path: path, Content: evo.Bytes("abc")}
	if err := advRun(t, evo.Config{}, desired.Write); err != nil {
		t.Fatalf("Write: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	advSeed(t, path, "abd") // same size
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := advRun(t, evo.Config{}, desired.Verify); !errors.Is(err, evo.ErrVerifyMismatch) {
		t.Fatalf("Verify after a same-size, same-mtime edit = %v, want ErrVerifyMismatch", err)
	}
}

// Structural: an already-current Write keeps the inode and mtime, so
// mtime-based tools see no churn.
func TestAdversarial_CurrentContentIsNotRewritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	desired := evo.File{Path: path, Content: evo.Bytes("same"), Mode: 0o644}
	if err := advRun(t, evo.Config{}, desired.Write); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	for range 5 {
		if err := advRun(t, evo.Config{}, desired.Write); err != nil {
			t.Fatal(err)
		}
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) || !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("current content was rewritten (inode or mtime changed)")
	}
}

// The destination lock is keyed on the file, not on how the caller spelled
// the path: dot segments and a symlinked directory alias must not split it.
func TestAdversarial_AliasedPathSpellingsShareOneLock(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(real, alias); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	spellings := []string{
		filepath.Join(real, "shared"),
		filepath.Join(real, ".", "shared"),
		filepath.Join(real, "..", "real", "shared"),
		filepath.Join(alias, "shared"),
	}
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: t.TempDir()})
	defer func() { _ = out.Close() }()
	group := out.Group("aliases")
	for i := range 16 {
		path := spellings[i%len(spellings)]
		body := strings.Repeat(string(rune('a'+i)), advHelperSize)
		group.Task(fmt.Sprintf("w%d", i)).Define(func(ctx context.Context) error {
			return evo.File{Path: path, Content: evo.Bytes(body)}.Write(ctx)
		})
	}
	if err := group.Wait(); err != nil {
		t.Fatalf("writers: %v", err)
	}
	_ = out.Finish()
	advRequireUniform(t, spellings[0])
}

func TestAdversarial_ZeroModeCreatesExactly0644UnderRestrictiveUmask(t *testing.T) {
	old := syscall.Umask(0o077)
	defer syscall.Umask(old)
	path := filepath.Join(t.TempDir(), "f")
	err := advRun(t, evo.Config{}, func(ctx context.Context) error {
		return evo.File{Path: path, Content: evo.Bytes("x")}.Write(ctx)
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("zero Mode created %v under umask 077, want 0644", info.Mode().Perm())
	}
}

func TestAdversarial_LargeBinaryContentRoundTrips(t *testing.T) {
	data := make([]byte, 8<<20)
	for i := range data {
		data[i] = byte(i*31 + i>>8) // every byte value, NULs included
	}
	path := filepath.Join(t.TempDir(), "big.bin")
	desired := evo.File{Path: path, Content: evo.Bytes(data)}
	var read []byte
	var sum string
	err := advRun(t, evo.Config{}, func(ctx context.Context) error {
		if err := desired.Write(ctx); err != nil {
			return err
		}
		if err := desired.Verify(ctx); err != nil {
			return err
		}
		var err error
		if read, err = desired.Read(ctx); err != nil {
			return err
		}
		sum, err = desired.Checksum(ctx)
		return err
	})
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	if !bytes.Equal(read, data) {
		t.Fatalf("Read returned %d bytes that differ from the %d written", len(read), len(data))
	}
	if sum != hexSum(data) {
		t.Fatalf("Checksum = %q, want %q", sum, hexSum(data))
	}
}

func TestAdversarial_UnusualFileNamesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"with space", "-leading-dash", "uni\u00e7ode-\u65e5\u672c", "semi;colon&amp", "new\nline", ".hidden", "a.b.c.tar.gz"} {
		path := filepath.Join(dir, name)
		f := evo.File{Path: path, Content: evo.Bytes("body of " + name)}
		err := advRun(t, evo.Config{}, func(ctx context.Context) error {
			if err := f.Write(ctx); err != nil {
				return err
			}
			return f.Verify(ctx)
		})
		if err != nil {
			t.Fatalf("name %q: %v", name, err)
		}
		if got := advRead(t, path); got != "body of "+name {
			t.Fatalf("name %q holds %q", name, got)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 7 {
		t.Fatalf("directory has %d entries, want exactly the 7 destinations", len(entries))
	}
}

// Replacement is by rename, so a read-only destination (a directory write
// bit is what matters) is replaced and a zero Mode preserves its mode.
func TestAdversarial_ReadOnlyFileIsReplacedAndKeepsItsMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	advSeed(t, path, "old")
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	err := advRun(t, evo.Config{}, func(ctx context.Context) error {
		return evo.File{Path: path, Content: evo.Bytes("new")}.Write(ctx)
	})
	if err != nil {
		t.Fatalf("Write over a 0444 file: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if advRead(t, path) != "new" || info.Mode().Perm() != 0o444 {
		t.Fatalf("content %q mode %v, want new / 0444", advRead(t, path), info.Mode().Perm())
	}
}

func TestAdversarial_CanceledWriteKeepsPreviousContentAndMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f")
	advSeed(t, path, "previous")
	err := advRun(t, evo.Config{}, func(ctx context.Context) error {
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		return evo.File{Path: path, Content: evo.Bytes("replacement"), Mode: 0o600}.Write(cctx)
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Write = %v, want context.Canceled", err)
	}
	info, statErr := os.Stat(path)
	if statErr != nil {
		t.Fatal(statErr)
	}
	if advRead(t, path) != "previous" || info.Mode().Perm() != 0o644 {
		t.Fatalf("canceled Write changed the destination: %q %v", advRead(t, path), info.Mode().Perm())
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Fatalf("canceled Write left %d entries, want only the destination", len(entries))
	}
}

func TestAdversarial_ExpiredDeadlinePublishesNothing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f")
	err := advRun(t, evo.Config{}, func(ctx context.Context) error {
		dctx, cancel := context.WithDeadline(ctx, time.Now().Add(-time.Second))
		defer cancel()
		return evo.File{Path: path, Content: evo.Bytes("x")}.Write(dctx)
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Write past its deadline = %v, want context.DeadlineExceeded", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("Write past its deadline left %d entries", len(entries))
	}
}

func TestAdversarial_DryRunWriteDoesNotChmod(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	advSeed(t, path, "same")
	err := advRun(t, evo.Config{DryRun: true}, func(ctx context.Context) error {
		return evo.File{Path: path, Content: evo.Bytes("same"), Mode: 0o600}.Write(ctx)
	})
	if err != nil {
		t.Fatalf("dry-run Write: %v", err)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o644 {
		t.Fatalf("dry-run Write changed the mode to %v", info.Mode().Perm())
	}
}

// File.Read racing a rewrite sees the whole old or whole new content.
func TestAdversarial_EvoReadNeverSeesTornContent(t *testing.T) {
	const size = 1 << 20
	path := filepath.Join(t.TempDir(), "blob")
	a, b := bytes.Repeat([]byte{'a'}, size), bytes.Repeat([]byte{'b'}, size)
	advSeed(t, path, string(a))
	var torn atomic.Value
	err := advRun(t, evo.Config{}, func(ctx context.Context) error {
		var done atomic.Bool
		var wg sync.WaitGroup
		wg.Go(func() {
			for !done.Load() {
				got, err := evo.File{Path: path}.Read(ctx)
				if err != nil {
					torn.Store(fmt.Sprintf("Read failed mid-write: %v", err))
					return
				}
				if !bytes.Equal(got, a) && !bytes.Equal(got, b) {
					torn.Store(fmt.Sprintf("Read observed %d bytes of mixed content", len(got)))
					return
				}
			}
		})
		defer func() { done.Store(true); wg.Wait() }()
		for i := range 30 {
			body := a
			if i%2 == 0 {
				body = b
			}
			if err := (evo.File{Path: path, Content: evo.Bytes(body)}).Write(ctx); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if msg := torn.Load(); msg != nil {
		t.Fatal(msg)
	}
}

func advRequireUniform(t *testing.T, path string) {
	t.Helper()
	got := advRead(t, path)
	if len(got) != advHelperSize || strings.Count(got, got[:1]) != len(got) {
		t.Fatalf("destination holds %d bytes that are not one writer's whole content", len(got))
	}
}

// advHelper re-executes this test binary as a writer process. rounds "0"
// writes forever (until killed).
func advHelper(t *testing.T, path, letter, rounds string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestAdversarial_HelperProcess$")
	cmd.Env = append(os.Environ(),
		advHelperEnv+"=1", advHelperPath+"="+path, advHelperLetter+"="+letter, advHelperRounds+"="+rounds)
	return cmd
}

// TestAdversarial_HelperProcess is not a test: it is the writer process
// advHelper spawns.
func TestAdversarial_HelperProcess(t *testing.T) {
	if os.Getenv(advHelperEnv) != "1" {
		return
	}
	path, letter := os.Getenv(advHelperPath), os.Getenv(advHelperLetter)
	var rounds int
	_, _ = fmt.Sscan(os.Getenv(advHelperRounds), &rounds)
	body := strings.Repeat(letter, advHelperSize)
	err := advRun(t, evo.Config{}, func(ctx context.Context) error {
		for i := 0; rounds == 0 || i < rounds; i++ {
			// Alternate so every round really commits.
			content := body
			if i%2 == 1 {
				content = strings.Repeat(strings.ToUpper(letter), advHelperSize)
			}
			if err := (evo.File{Path: path, Content: evo.Bytes(content)}).Write(ctx); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func BenchmarkFileWrite_4KiBChanged(b *testing.B) {
	path := filepath.Join(b.TempDir(), "f")
	bodies := [2][]byte{bytes.Repeat([]byte{'a'}, 4<<10), bytes.Repeat([]byte{'b'}, 4<<10)}
	b.SetBytes(4 << 10)
	err := advRun(b, evo.Config{}, func(ctx context.Context) error {
		b.ResetTimer()
		for i := 0; b.Loop(); i++ {
			if err := (evo.File{Path: path, Content: evo.Bytes(bodies[i%2])}).Write(ctx); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		b.Fatal(err)
	}
}

func BenchmarkFileWrite_AlreadyCurrent(b *testing.B) {
	path := filepath.Join(b.TempDir(), "f")
	body := bytes.Repeat([]byte{'a'}, 4<<10)
	err := advRun(b, evo.Config{}, func(ctx context.Context) error {
		desired := evo.File{Path: path, Content: evo.Bytes(body)}
		if err := desired.Write(ctx); err != nil {
			return err
		}
		b.ResetTimer()
		for b.Loop() {
			if err := desired.Write(ctx); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		b.Fatal(err)
	}
}
