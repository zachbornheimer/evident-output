package exec_test

// Adversarial hardening tests for evo.Exec. Each test is named for the
// weakness it guards; sources are in docs/zys-1382/adversarial-research.md.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

const (
	advLiveness  = 10 * time.Second
	advHelperEnv = "EVO_ADV_EXEC_HELPER"
	advHelperURL = "EVO_ADV_EXEC_URL"
)

func advRun(tb testing.TB, cfg evo.Config, fn func(context.Context) error) error {
	tb.Helper()
	cfg.Isolated, cfg.Plain = true, true
	if cfg.Stdout == nil {
		cfg.Stdout = io.Discard
	}
	if cfg.Stderr == nil {
		cfg.Stderr = io.Discard
	}
	if cfg.StateDir == "" {
		cfg.StateDir = tb.TempDir()
	}
	out := evo.Init(cfg)
	defer func() { _ = out.Close() }()
	err := out.Task("adversarial").Define(fn).Wait()
	_ = out.Finish()
	return err
}

// advExec runs one Exec and returns its result and error, failing the test
// when the call outlives advLiveness.
func advExec(t *testing.T, cfg evo.Config, ctxTimeout time.Duration, x evo.Exec) (evo.ExecResult, error) {
	t.Helper()
	type outcome struct {
		res evo.ExecResult
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		var res evo.ExecResult
		err := advRun(t, cfg, func(ctx context.Context) error {
			if ctxTimeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, ctxTimeout)
				defer cancel()
			}
			var err error
			res, err = x.Run(ctx)
			return err
		})
		done <- outcome{res, err}
	}()
	select {
	case o := <-done:
		return o.res, o.err
	case <-time.After(advLiveness):
		t.Fatalf("Exec %s did not return within %v", x.Path, advLiveness)
		return evo.ExecResult{}, nil
	}
}

func advSh(dir, script string) evo.Exec {
	return evo.Exec{Path: "/bin/sh", Args: []string{"-c", script}, Dir: dir}
}

func advAlive(pid int) bool { return syscall.Kill(pid, 0) == nil }

func advWaitGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(advLiveness)
	for advAlive(pid) {
		if time.Now().After(deadline) {
			_ = syscall.Kill(pid, syscall.SIGTERM)
			t.Fatalf("grandchild %d outlived Exec's cancellation", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

type advSyncBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *advSyncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *advSyncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type advRedactor struct{ secret string }

func (r advRedactor) RedactString(s string) string {
	return strings.ReplaceAll(s, r.secret, "[redacted]")
}

func TestAdversarial_ArgsNeverInterpretedByShell(t *testing.T) {
	dir := t.TempDir()
	hostile := "$(touch pwned1); `touch pwned2`; touch pwned3 && echo *"
	res, err := advExec(t, evo.Config{}, 0, evo.Exec{Path: "/usr/bin/printf", Args: []string{"%s", hostile}, Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if res.Stdout != hostile {
		t.Fatalf("argument was rewritten: %q", res.Stdout)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("argument was executed by a shell: %s exists", entries[0].Name())
	}
}

// Go 1.19 exec.ErrDot: a bare name must never resolve through a relative
// PATH entry to a binary in the working directory.
func TestAdversarial_RelativePathEntryNotResolved(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "hijacked")
	tool := filepath.Join(dir, "evo-adv-tool")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\ntouch "+marker+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{".", "", "./"} {
		_, err := advExec(t, evo.Config{}, 0, evo.Exec{Path: "evo-adv-tool", Dir: dir, Env: []string{"PATH=" + path}})
		if !errors.Is(err, evo.ErrExecExecutableNotFound) {
			t.Fatalf("bare name through PATH=%q = %v, want ErrExecExecutableNotFound", path, err)
		}
		if _, statErr := os.Stat(marker); statErr == nil {
			t.Fatalf("PATH=%q ran a binary from the working directory", path)
		}
	}
}

func TestAdversarial_CancellationKillsProcessGroup(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "pid")
	script := "sleep 60 & echo $! > " + pidFile + "; wait"
	var grandchild int
	errCh := make(chan error, 1)
	go func() {
		errCh <- advRun(t, evo.Config{}, func(ctx context.Context) error {
			cctx, cancel := context.WithCancel(ctx)
			defer cancel()
			go func() {
				deadline := time.Now().Add(advLiveness)
				for time.Now().Before(deadline) {
					if b, err := os.ReadFile(pidFile); err == nil && strings.TrimSpace(string(b)) != "" {
						grandchild, _ = strconv.Atoi(strings.TrimSpace(string(b)))
						break
					}
					time.Sleep(10 * time.Millisecond)
				}
				cancel()
			}()
			_, err := advSh(dir, script).Run(cctx)
			return err
		})
	}()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled Exec = %v, want context.Canceled", err)
		}
	case <-time.After(2 * advLiveness):
		t.Fatal("canceled Exec did not return")
	}
	if grandchild == 0 {
		t.Fatal("grandchild never reported its pid")
	}
	advWaitGone(t, grandchild)
}

// Go issue 23019 (WaitDelay): a background grandchild holding stdout must
// not keep Exec waiting after the direct child exits.
func TestAdversarial_GrandchildHoldingPipesDoesNotHang(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "pid")
	res, err := advExec(t, evo.Config{}, 0, advSh(dir, "sleep 30 & echo $! > "+pidFile+"; exit 0"))
	if b, readErr := os.ReadFile(pidFile); readErr == nil {
		if pid, _ := strconv.Atoi(strings.TrimSpace(string(b))); pid > 0 {
			_ = syscall.Kill(pid, syscall.SIGTERM)
		}
	}
	if !res.Ran || res.ExitCode != 0 || errors.Is(err, evo.ErrExecNonzeroExit) {
		t.Fatalf("direct child exited 0 but Exec reported %+v, %v", res, err)
	}
}

// exec.CommandContext's default kill reaches only the direct child; a
// deadline must end the whole process group, same as cancellation.
func TestAdversarial_DeadlineKillsProcessGroup(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "pid")
	_, err := advExec(t, evo.Config{}, 500*time.Millisecond, advSh(dir, "sleep 60 & echo $! > "+pidFile+"; wait"))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Exec past its deadline = %v, want context.DeadlineExceeded", err)
	}
	b, readErr := os.ReadFile(pidFile)
	if readErr != nil {
		t.Fatalf("grandchild never reported its pid: %v", readErr)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	if pid <= 0 {
		t.Fatalf("grandchild pid file = %q", b)
	}
	advWaitGone(t, pid)
}

// Structural: a 64 MiB flood is captured within a bounded tail.
func TestAdversarial_OutputFloodIsBounded(t *testing.T) {
	const flood = 64 << 20
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	res, err := advExec(t, evo.Config{}, 0, advSh(t.TempDir(), fmt.Sprintf("head -c %d /dev/zero | tr '\\0' a; head -c %d /dev/zero | tr '\\0' b >&2", flood, flood)))
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Truncated {
		t.Fatal("a 64 MiB flood was not reported as truncated")
	}
	if len(res.Stdout)+len(res.Stderr) > 2<<20 {
		t.Fatalf("Exec retained %d bytes of output", len(res.Stdout)+len(res.Stderr))
	}
	if grew := int64(after.HeapInuse) - int64(before.HeapInuse); grew > flood/4 {
		t.Fatalf("heap grew %d MiB while capturing a flood", grew>>20)
	}
}

// The parent's stdin is replaced with a pipe holding data, so a child
// wired to os.Stdin would echo it back instead of seeing end-of-file.
func TestAdversarial_StdinIsNotInherited(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString("leaked-parent-stdin\n"); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	saved := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = saved; _ = r.Close() })
	res, err := advExec(t, evo.Config{}, 0, evo.Exec{Path: "/bin/cat", Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("cat with no declared stdin: %v", err)
	}
	if res.Stdout != "" {
		t.Fatalf("child read stdin it was never given: %q", res.Stdout)
	}
}

func TestAdversarial_SignalDeathIsFailure(t *testing.T) {
	res, err := advExec(t, evo.Config{}, 0, advSh(t.TempDir(), "kill -KILL $$"))
	if err == nil {
		t.Fatal("a child killed by SIGKILL was reported as success")
	}
	if res.ExitCode == 0 {
		t.Fatal("a signal death carries exit code 0")
	}
}

func TestAdversarial_DeadlineIsClassifiedAsDeadline(t *testing.T) {
	_, err := advExec(t, evo.Config{}, 200*time.Millisecond, advSh(t.TempDir(), "sleep 30"))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Exec past its deadline = %v, want context.DeadlineExceeded", err)
	}
	if errors.Is(err, evo.ErrExecNonzeroExit) {
		t.Fatal("a deadline kill was classified as an ordinary nonzero exit")
	}
}

func TestAdversarial_DuplicateEnvLastWins(t *testing.T) {
	res, err := advExec(t, evo.Config{}, 0, evo.Exec{
		Path: "/usr/bin/printenv", Args: []string{"EVO_ADV"}, Dir: t.TempDir(),
		Env: []string{"EVO_ADV=first", "EVO_ADV=last"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(res.Stdout); got != "last" {
		t.Fatalf("EVO_ADV = %q, want last", got)
	}
}

func TestAdversarial_SecretsRedactedInEvidence(t *testing.T) {
	const secret = "s3kr3t-value"
	var rendered advSyncBuffer
	cfg := evo.Config{Redactor: advRedactor{secret}, Stdout: &rendered, Stderr: &rendered, Verbosity: evo.VerbosityVerbose}
	res, err := advExec(t, cfg, 0, evo.Exec{
		// The secret arrives through Env and through Args, so neither the
		// captured streams nor a rendered command line may carry it.
		Path: "/bin/sh", Args: []string{"-c", `echo "$TOKEN"; echo "$1" >&2; exit 3`, "sh", secret},
		Dir: t.TempDir(), Env: []string{"TOKEN=" + secret},
	})
	if err == nil {
		t.Fatal("exit 3 reported success")
	}
	for name, text := range map[string]string{"Stdout": res.Stdout, "Stderr": res.Stderr, "error": err.Error(), "rendered output": rendered.String()} {
		if strings.Contains(text, secret) {
			t.Fatalf("%s leaks the secret", name)
		}
	}
}

func TestAdversarial_MissingExecutableIsNotAnExit(t *testing.T) {
	res, err := advExec(t, evo.Config{}, 0, evo.Exec{Path: filepath.Join(t.TempDir(), "absent"), Dir: t.TempDir()})
	if err == nil {
		t.Fatal("a missing executable reported success")
	}
	if errors.Is(err, evo.ErrExecNonzeroExit) || res.Ran {
		t.Fatalf("a missing executable was classified as a process exit: %v", err)
	}
}

// A spawn that fails before the child exists is never an exit status:
// os/exec returns a start error (EACCES, ENOENT on Dir, EINVAL for a NUL
// byte) where a careless wrapper reports exit -1 or 127.
func TestAdversarial_SpawnFailuresAreNotExits(t *testing.T) {
	notExecutable := filepath.Join(t.TempDir(), "data.txt")
	if err := os.WriteFile(notExecutable, []byte("#!/bin/sh\nexit 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := map[string]evo.Exec{
		"Path not executable": {Path: notExecutable, Dir: t.TempDir()},
		"Path is a directory": {Path: t.TempDir(), Dir: t.TempDir()},
		"Dir does not exist":  {Path: "/bin/sh", Args: []string{"-c", "exit 0"}, Dir: filepath.Join(t.TempDir(), "absent")},
		"NUL byte in Args":    {Path: "/usr/bin/printf", Args: []string{"a\x00b"}, Dir: t.TempDir()},
		"NUL byte in Env":     {Path: "/usr/bin/true", Env: []string{"EVO_ADV=a\x00b"}, Dir: t.TempDir()},
	}
	for name, x := range cases {
		t.Run(name, func(t *testing.T) {
			res, err := advExec(t, evo.Config{}, 0, x)
			if err == nil {
				t.Fatal("a spawn that cannot start reported success")
			}
			if res.Ran || errors.Is(err, evo.ErrExecNonzeroExit) {
				t.Fatalf("a spawn failure was classified as a process exit: %+v, %v", res, err)
			}
		})
	}
}

// A context already done must not start a child it would immediately kill.
func TestAdversarial_DoneContextNeverSpawns(t *testing.T) {
	dir := t.TempDir()
	res, err := advExec(t, evo.Config{}, 0, evo.Exec{Path: "/usr/bin/touch", Args: []string{filepath.Join(dir, "spawned")}, Dir: dir})
	if err != nil {
		t.Fatalf("control run: %v", err)
	}
	_ = os.Remove(filepath.Join(dir, "spawned"))
	err = advRun(t, evo.Config{}, func(ctx context.Context) error {
		ctx, cancel := context.WithCancel(ctx)
		cancel()
		res, err = evo.Exec{Path: "/usr/bin/touch", Args: []string{filepath.Join(dir, "spawned")}, Dir: dir}.Run(ctx)
		return err
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run on a cancelled ctx = %v, want context.Canceled", err)
	}
	if res.Ran {
		t.Fatal("Run on a cancelled ctx reports Ran")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "spawned")); statErr == nil {
		t.Fatal("Run on a cancelled ctx spawned the child")
	}
}

// os.Chdir is process-wide; Dir must be applied to the child only.
func TestAdversarial_DirDoesNotChangeParentWorkingDirectory(t *testing.T) {
	before, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	res, err := advExec(t, evo.Config{}, 0, evo.Exec{Path: "/bin/pwd", Args: []string{"-P"}, Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(res.Stdout); got != dir {
		t.Fatalf("child ran in %q, want %q", got, dir)
	}
	if after, _ := os.Getwd(); after != before {
		t.Fatalf("Exec changed the parent's working directory to %q", after)
	}
}

// Structural: one Run spawns exactly one process.
func TestAdversarial_OneSpawnPerCall(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "spawns")
	if _, err := advExec(t, evo.Config{}, 0, advSh(dir, "echo spawn >> "+log)); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(b), "spawn\n"); n != 1 {
		t.Fatalf("one Exec spawned %d processes", n)
	}
}

// Structural: concurrent Execs never exceed the Run's concurrency bound.
// Each child is this test binary, reporting to an HTTP endpoint that tracks
// how many children are alive at once.
func TestAdversarial_ConcurrentExecsAreBounded(t *testing.T) {
	const bound, children = 3, 12
	var mu sync.Mutex
	alive, peak := 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		alive++
		peak = max(peak, alive)
		mu.Unlock()
		time.Sleep(50 * time.Millisecond)
		mu.Lock()
		alive--
		mu.Unlock()
	}))
	defer srv.Close()
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: t.TempDir(), MaxConcurrency: bound})
	defer func() { _ = out.Close() }()
	group := out.Group("children")
	for i := range children {
		x := evo.Exec{
			Path: os.Args[0], Args: []string{"-test.run=^TestAdversarial_HelperProcess$"}, Dir: t.TempDir(),
			Env: append(os.Environ(), advHelperEnv+"=1", advHelperURL+"="+srv.URL),
		}
		group.Task(fmt.Sprintf("c%d", i)).Define(func(ctx context.Context) error {
			_, err := x.Run(ctx)
			return err
		})
	}
	if err := group.Wait(); err != nil {
		t.Fatal(err)
	}
	_ = out.Finish()
	if peak > bound {
		t.Fatalf("peak concurrent children = %d, bound %d", peak, bound)
	}
}

// TestAdversarial_HelperProcess is not a test: it is the child
// TestAdversarial_ConcurrentExecsAreBounded spawns.
func TestAdversarial_HelperProcess(t *testing.T) {
	if os.Getenv(advHelperEnv) != "1" {
		return
	}
	resp, err := http.Get(os.Getenv(advHelperURL))
	if err != nil {
		os.Exit(2)
	}
	_ = resp.Body.Close()
	os.Exit(0)
}

func BenchmarkExec_True(b *testing.B) {
	err := advRun(b, evo.Config{}, func(ctx context.Context) error {
		x := evo.Exec{Path: "/usr/bin/true", Dir: b.TempDir()}
		for b.Loop() {
			if _, err := x.Run(ctx); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		b.Fatal(err)
	}
}
