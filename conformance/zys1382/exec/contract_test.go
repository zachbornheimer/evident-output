// Package exec_test is the ZYS-1382 execution contract for evo.Exec: the
// standard process vocabulary (Path, Args, Dir, Env, Outputs) with Evo
// owning lifetime, capture, cancellation, exit classification, redaction,
// evidence, and output verification. Names and open-syntax choices:
// docs/zys-1382/contract-decisions.md.
//
// Reference vocabularies the shape is checked against: Go os/exec.Cmd
// (Path/Args/Dir/Env; nil Env inherits, empty Env is clean), Python
// subprocess (args/cwd/env), Rust std::process::Command
// (program/args/current_dir/env), Node child_process (file/args/cwd/env).
// Unlike os/exec.Cmd.Args, Exec.Args excludes argv[0] (as Rust and Node do).
package exec_test

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

// promptly bounds how long a cancelled child may outlive its cancellation.
const promptly = 10 * time.Second

// contractRun runs fn inside one Task's Define callback on an isolated
// Output and returns fn's own error.
func contractRun(t *testing.T, cfg evo.Config, fn func(ctx context.Context) error) error {
	t.Helper()
	cfg.Isolated, cfg.Plain = true, true
	if cfg.Stdout == nil {
		cfg.Stdout = io.Discard
	}
	if cfg.Stderr == nil {
		cfg.Stderr = io.Discard
	}
	if cfg.StateDir == "" {
		cfg.StateDir = t.TempDir()
	}
	out := evo.Init(cfg)
	var (
		ran bool
		got error
	)
	task := out.Task("contract").Define(func(ctx context.Context) error {
		ran = true
		got = fn(ctx)
		return got
	})
	_ = task.Wait()
	_ = out.Finish()
	if !ran {
		t.Fatalf("contract Task callback never ran")
	}
	return got
}

// plant writes files (slash-separated relative path -> content) under root.
func plant(t *testing.T, root string, files map[string]string) {
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
}

// tool writes an executable /bin/sh script named name into dir.
func tool(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// sh builds an Exec that runs script under /bin/sh in dir.
func sh(dir, script string) evo.Exec {
	return evo.Exec{Path: "/bin/sh", Args: []string{"-c", script}, Dir: dir}
}

func run(t *testing.T, cfg evo.Config, x evo.Exec) (evo.ExecResult, error) {
	t.Helper()
	var result evo.ExecResult
	err := contractRun(t, cfg, func(ctx context.Context) error {
		var err error
		result, err = x.Run(ctx)
		return err
	})
	return result, err
}

func absent(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Lstat(path)
	return errors.Is(err, fs.ErrNotExist)
}

type syncBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// Field types are pinned at compile time; the exact field set is pinned
// in removed_test.go.
func TestExecIsAPlainStructLiteralInProcessVocabulary(t *testing.T) {
	x := evo.Exec{
		Path:    "python3",
		Args:    []string{"generate.py", "input.xlsx", "out.bin"},
		Dir:     "/work",
		Env:     []string{"PYTHONHASHSEED=0"},
		Outputs: evo.Outputs{evo.File{Path: "out.bin"}, evo.Tree{Path: "build"}},
	}
	// The result types pin each field's declared type.
	fieldTypes := func(x evo.Exec) (string, []string, string, []string, evo.Outputs) {
		return x.Path, x.Args, x.Dir, x.Env, x.Outputs
	}
	_, _, _, _, _ = fieldTypes(x)
	var zero evo.Exec
	if zero.Path != "" || zero.Args != nil || zero.Env != nil || zero.Outputs != nil {
		t.Fatalf("zero Exec is not empty: %+v", zero)
	}
	if x.Path != "python3" || len(x.Args) != 3 || x.Dir != "/work" || len(x.Env) != 1 || len(x.Outputs) != 2 {
		t.Fatalf("Exec literal did not round-trip: %+v", x)
	}
	if _, ok := x.Outputs[0].(evo.File); !ok {
		t.Fatalf("Outputs[0] = %T, want evo.File", x.Outputs[0])
	}
	if _, ok := x.Outputs[1].(evo.Tree); !ok {
		t.Fatalf("Outputs[1] = %T, want evo.Tree", x.Outputs[1])
	}
}

func TestExecRunsTheProcessAndReportsExit(t *testing.T) {
	cases := []struct {
		name   string
		script string
		code   int
	}{
		{"exit 0", "exit 0", 0},
		{"exit 3", "exit 3", 3},
		{"exit 1", "false", 1},
		{"exit 255", "exit 255", 255},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := run(t, evo.Config{}, sh(t.TempDir(), tc.script))
			if tc.code == 0 && err != nil {
				t.Fatalf("Run = %v, want nil for a zero exit", err)
			}
			if tc.code != 0 && !errors.Is(err, evo.ErrExecNonzeroExit) {
				t.Fatalf("Run = %v, want ErrExecNonzeroExit", err)
			}
			if !result.Ran || result.ExitCode != tc.code {
				t.Fatalf("result = %+v, want Ran with ExitCode %d", result, tc.code)
			}
		})
	}
}

// Args are argv[1:]: passed literally, one element per argument, with
// empty strings, spaces, and newlines preserved. Path is argv[0]'s program.
func TestExecArgsAreTheArgumentsAfterTheProgram(t *testing.T) {
	x := evo.Exec{Path: "/usr/bin/printf", Args: []string{"%s|", "", "a b", "c\nd"}, Dir: t.TempDir()}
	result, err := run(t, evo.Config{}, x)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if want := "|a b|c\nd|"; result.Stdout != want {
		t.Fatalf("Stdout = %q, want %q (Args[0] must be the first argument, not argv[0])", result.Stdout, want)
	}
}

func TestExecCapturesStdoutAndStderrSeparately(t *testing.T) {
	result, err := run(t, evo.Config{}, sh(t.TempDir(), `printf 'out1\nout2\n'; printf 'err1\n' >&2`))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.TrimSpace(result.Stdout) != "out1\nout2" {
		t.Fatalf("Stdout = %q", result.Stdout)
	}
	if strings.TrimSpace(result.Stderr) != "err1" {
		t.Fatalf("Stderr = %q", result.Stderr)
	}
	if result.Truncated {
		t.Fatalf("three lines reported as Truncated")
	}
}

func TestExecNonzeroExitStillReturnsTheCapturedResult(t *testing.T) {
	result, err := run(t, evo.Config{}, sh(t.TempDir(), `echo 'file.go:10: unused variable'; echo 'lint failed' >&2; exit 1`))
	if !errors.Is(err, evo.ErrExecNonzeroExit) {
		t.Fatalf("Run = %v, want ErrExecNonzeroExit", err)
	}
	if !strings.Contains(result.Stdout, "file.go:10: unused variable") {
		t.Fatalf("Stdout after a nonzero exit = %q, want the linter's finding", result.Stdout)
	}
	if !strings.Contains(result.Stderr, "lint failed") {
		t.Fatalf("Stderr after a nonzero exit = %q", result.Stderr)
	}
	if !result.Ran || result.ExitCode != 1 {
		t.Fatalf("result = %+v, want Ran with ExitCode 1", result)
	}
}

func TestExecRunsInDir(t *testing.T) {
	dir := t.TempDir()
	result, err := run(t, evo.Config{}, sh(dir, "pwd"))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, _ := filepath.EvalSymlinks(strings.TrimSpace(result.Stdout))
	want, _ := filepath.EvalSymlinks(dir)
	if got != want {
		t.Fatalf("pwd = %q, want %q", got, want)
	}
}

// Env follows os/exec: nil inherits the parent, a non-nil slice (even an
// empty one) is the child's whole environment, and appending to
// os.Environ() is how a caller adds to the inherited set.
func TestExecEnvIsTheChildEnvironment(t *testing.T) {
	t.Setenv("ZYS1382_PROBE", "inherited")
	cases := []struct {
		name string
		env  []string
		want string
	}{
		{"nil Env inherits the parent", nil, "inherited|unset"},
		{"explicit Env replaces the parent", []string{"ZYS1382_OTHER=explicit"}, "unset|explicit"},
		{"empty non-nil Env is a clean environment", []string{}, "unset|unset"},
		{"os.Environ plus entries inherits and overrides", append(os.Environ(), "ZYS1382_PROBE=override", "ZYS1382_OTHER=added"), "override|added"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			x := sh(t.TempDir(), `printf '%s|%s' "${ZYS1382_PROBE-unset}" "${ZYS1382_OTHER-unset}"`)
			x.Env = tc.env
			result, err := run(t, evo.Config{}, x)
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if result.Stdout != tc.want {
				t.Fatalf("child saw %q, want %q", result.Stdout, tc.want)
			}
		})
	}
}

func TestExecResolvesPathThroughPATH(t *testing.T) {
	x := evo.Exec{Path: "sh", Args: []string{"-c", "exit 0"}, Dir: t.TempDir()}
	if _, err := run(t, evo.Config{}, x); err != nil {
		t.Fatalf("Run with a bare Path = %v", err)
	}
}

// A bare Path resolves through the PATH the child will run with: the
// parent's when Env is nil, Env's PATH entry when Env is set (Python
// subprocess, Rust Command, and Node child_process behave this way;
// os/exec alone looks up in the parent).
func TestExecBarePathResolvesThroughTheChildPATH(t *testing.T) {
	const name = "zys1382-custom-path-tool"
	bin := t.TempDir()
	tool(t, bin, name, "printf custom")
	system := "/usr/bin:/bin"

	t.Run("custom PATH in Env", func(t *testing.T) {
		x := evo.Exec{Path: name, Dir: t.TempDir(), Env: []string{"PATH=" + bin + ":" + system}}
		result, err := run(t, evo.Config{}, x)
		if err != nil {
			t.Fatalf("Run with Env PATH naming the tool's dir = %v", err)
		}
		if result.Stdout != "custom" {
			t.Fatalf("Stdout = %q, want custom", result.Stdout)
		}
	})
	t.Run("parent PATH when Env is nil", func(t *testing.T) {
		t.Setenv("PATH", bin+":"+system)
		x := evo.Exec{Path: name, Dir: t.TempDir()}
		if _, err := run(t, evo.Config{}, x); err != nil {
			t.Fatalf("Run with the parent PATH naming the tool's dir = %v", err)
		}
	})
	t.Run("Env PATH overrides the parent PATH", func(t *testing.T) {
		t.Setenv("PATH", bin+":"+system)
		x := evo.Exec{Path: name, Dir: t.TempDir(), Env: []string{"PATH=" + system}}
		result, err := run(t, evo.Config{}, x)
		if !errors.Is(err, evo.ErrExecExecutableNotFound) {
			t.Fatalf("Run = %v, want ErrExecExecutableNotFound (only the parent PATH has the tool)", err)
		}
		if result.Ran {
			t.Fatalf("result reports Ran for a process that never spawned")
		}
	})
}

// A Path containing a separator is not looked up on PATH; a relative one
// resolves against Dir (os/exec Cmd.Path semantics).
func TestExecRelativePathWithSeparatorResolvesAgainstDir(t *testing.T) {
	dir := t.TempDir()
	tool(t, dir, "local-tool", "printf local")
	result, err := run(t, evo.Config{}, evo.Exec{Path: "./local-tool", Dir: dir})
	if err != nil {
		t.Fatalf("Run ./local-tool in Dir = %v", err)
	}
	if result.Stdout != "local" {
		t.Fatalf("Stdout = %q, want local", result.Stdout)
	}
}

func TestExecMissingExecutableIsClassified(t *testing.T) {
	cases := map[string]string{
		"bare name not on PATH": "zys1382-definitely-not-installed",
		"absolute path absent":  filepath.Join(t.TempDir(), "absent"),
	}
	for name, path := range cases {
		t.Run(name, func(t *testing.T) {
			result, err := run(t, evo.Config{}, evo.Exec{Path: path, Dir: t.TempDir()})
			if !errors.Is(err, evo.ErrExecExecutableNotFound) {
				t.Fatalf("Run = %v, want ErrExecExecutableNotFound", err)
			}
			if result.Ran {
				t.Fatalf("result reports Ran for a process that never spawned")
			}
		})
	}
}

func TestExecRequiresAPath(t *testing.T) {
	result, err := run(t, evo.Config{}, evo.Exec{Args: []string{"-c", "true"}, Dir: t.TempDir()})
	if !errors.Is(err, evo.ErrExecPathMissing) {
		t.Fatalf("Run with empty Path = %v, want ErrExecPathMissing", err)
	}
	if result.Ran {
		t.Fatalf("result reports Ran with an empty Path")
	}
}

func TestExecRequiresATaskContext(t *testing.T) {
	dir := t.TempDir()
	result, err := sh(dir, "touch ran").Run(context.Background())
	if !errors.Is(err, evo.ErrNoTaskContext) {
		t.Fatalf("Run outside a Task = %v, want ErrNoTaskContext", err)
	}
	if result.Ran || !absent(t, filepath.Join(dir, "ran")) {
		t.Fatalf("Run outside a Task spawned the process")
	}
}

func TestExecOutputsVerifyGeneratedState(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.bin")
	gen := filepath.Join(dir, "gen")
	cases := []struct {
		name    string
		script  string
		outputs evo.Outputs
		wantErr error
	}{
		{"declared File produced", "printf gen > out.bin", evo.Outputs{evo.File{Path: out}}, nil},
		{"declared Tree produced", "mkdir -p gen/sub && printf x > gen/sub/a", evo.Outputs{evo.Tree{Path: gen}}, nil},
		{"File and Tree both produced", "printf gen > out.bin && mkdir -p gen && printf x > gen/a", evo.Outputs{evo.File{Path: out}, evo.Tree{Path: gen}}, nil},
		{"declared File missing after exit 0", "true", evo.Outputs{evo.File{Path: out}}, evo.ErrExecOutputMissingAfterSuccess},
		{"declared Tree missing after exit 0", "true", evo.Outputs{evo.Tree{Path: gen}}, evo.ErrExecOutputMissingAfterSuccess},
		{"one of two Outputs missing", "printf gen > out.bin", evo.Outputs{evo.File{Path: out}, evo.Tree{Path: gen}}, evo.ErrExecOutputMissingAfterSuccess},
		{"relative Output resolves against Dir", "printf gen > out.bin", evo.Outputs{evo.File{Path: "out.bin"}}, nil},
		{"relative Tree Output resolves against Dir", "mkdir -p gen && printf x > gen/a", evo.Outputs{evo.Tree{Path: "gen"}}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_ = os.RemoveAll(out)
			_ = os.RemoveAll(gen)
			x := sh(dir, tc.script)
			x.Outputs = tc.outputs
			result, err := run(t, evo.Config{}, x)
			if tc.wantErr == nil && err != nil {
				t.Fatalf("Run = %v", err)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("Run = %v, want %v", err, tc.wantErr)
			}
			if !result.Ran {
				t.Fatalf("the process did not run")
			}
		})
	}
}

// Outputs are typed: a File Output is satisfied only by a regular file and
// a Tree Output only by a directory.
func TestExecOutputOfTheWrongKindFails(t *testing.T) {
	cases := []struct {
		name    string
		script  string
		outputs func(dir string) evo.Outputs
	}{
		{"File declared, directory produced", "mkdir out", func(d string) evo.Outputs { return evo.Outputs{evo.File{Path: filepath.Join(d, "out")}} }},
		{"Tree declared, file produced", "printf x > out", func(d string) evo.Outputs { return evo.Outputs{evo.Tree{Path: filepath.Join(d, "out")}} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			x := sh(dir, tc.script)
			x.Outputs = tc.outputs(dir)
			result, err := run(t, evo.Config{}, x)
			if err == nil {
				t.Fatalf("Run = nil for an Output of the wrong kind")
			}
			if errors.Is(err, evo.ErrExecNonzeroExit) || !result.Ran || result.ExitCode != 0 {
				t.Fatalf("exit 0 with a wrong-kind Output classified as a process failure: %+v, %v", result, err)
			}
		})
	}
}

// A failed process is classified by its exit, not by the Outputs it never
// produced.
func TestExecNonzeroExitIsNotAnOutputFailure(t *testing.T) {
	dir := t.TempDir()
	x := sh(dir, "exit 2")
	x.Outputs = evo.Outputs{evo.File{Path: filepath.Join(dir, "never")}}
	_, err := run(t, evo.Config{}, x)
	if !errors.Is(err, evo.ErrExecNonzeroExit) {
		t.Fatalf("Run = %v, want ErrExecNonzeroExit", err)
	}
	if errors.Is(err, evo.ErrExecOutputMissingAfterSuccess) {
		t.Fatalf("a nonzero exit was reported as a missing Output: %v", err)
	}
}

// Outputs are desired state the process produces; when a declared Output
// names Content, Run verifies the produced state against it.
func TestExecOutputWithDeclaredContentIsVerified(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.txt")
	x := sh(dir, "printf wrong > out.txt")
	x.Outputs = evo.Outputs{evo.File{Path: out, Content: evo.Bytes("expected")}}
	if _, err := run(t, evo.Config{}, x); !errors.Is(err, evo.ErrVerifyMismatch) {
		t.Fatalf("Run = %v, want ErrVerifyMismatch for an Output whose content differs", err)
	}
	x = sh(dir, "printf expected > out.txt")
	x.Outputs = evo.Outputs{evo.File{Path: out, Content: evo.Bytes("expected")}}
	if _, err := run(t, evo.Config{}, x); err != nil {
		t.Fatalf("Run = %v for an Output whose content matches", err)
	}
}

// Outputs exist so Task freshness can verify generated state: a Task whose
// Basis is unchanged is current only while its Exec Outputs still hold
// what the last run produced.
func TestExecOutputsParticipateInTaskFreshness(t *testing.T) {
	work, state := t.TempDir(), t.TempDir()
	plant(t, work, map[string]string{"input.txt": "v1"})
	input, out := filepath.Join(work, "input.txt"), filepath.Join(work, "out.txt")
	pass := func() bool {
		t.Helper()
		o := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: state})
		ran := false
		task := o.Task("generate").Basis(evo.File{Path: input}).Define(func(ctx context.Context) error {
			ran = true
			x := sh(work, "cp input.txt out.txt")
			x.Outputs = evo.Outputs{evo.File{Path: out}}
			_, err := x.Run(ctx)
			return err
		})
		if err := task.Wait(); err != nil {
			t.Fatalf("generate: %v", err)
		}
		_ = o.Finish()
		return ran
	}
	steps := []struct {
		name    string
		prepare func()
		wantRun bool
	}{
		{"first run", func() {}, true},
		{"unchanged Basis and Outputs", func() {}, false},
		{"Output deleted", func() { _ = os.Remove(out) }, true},
		{"restored and unchanged", func() {}, false},
		{"Output tampered", func() { plant(t, work, map[string]string{"out.txt": "tampered"}) }, true},
	}
	for _, step := range steps {
		step.prepare()
		if got := pass(); got != step.wantRun {
			t.Fatalf("%s: Task ran = %v, want %v", step.name, got, step.wantRun)
		}
	}
}

func TestExecHonorsContextCancellation(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "pid")
	var child int
	start := time.Now()
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		go func() {
			for deadline := time.Now().Add(promptly); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
				if b, err := os.ReadFile(pidFile); err == nil && strings.HasSuffix(string(b), "\n") {
					child, _ = strconv.Atoi(strings.TrimSpace(string(b)))
					break
				}
			}
			cancel()
		}()
		_, err := sh(dir, "echo $$ > pid; exec sleep 30").Run(ctx)
		return err
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Run = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(start); elapsed > promptly {
		t.Fatalf("cancelled Run returned after %v", elapsed)
	}
	if child == 0 {
		t.Fatal("child never reported its pid")
	}
	if syscall.Kill(child, 0) == nil {
		_ = syscall.Kill(child, syscall.SIGKILL)
		t.Fatalf("child %d still alive after Run returned from cancellation", child)
	}
}

// A timeout is a context deadline (os/exec CommandContext, Python
// subprocess timeout=, Node child_process timeout): the child is ended and
// Run reports context.DeadlineExceeded, not an ordinary nonzero exit.
func TestExecHonorsContextDeadline(t *testing.T) {
	start := time.Now()
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
		defer cancel()
		_, err := sh(t.TempDir(), "exec sleep 30").Run(ctx)
		return err
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run past its deadline = %v, want context.DeadlineExceeded", err)
	}
	if errors.Is(err, evo.ErrExecNonzeroExit) {
		t.Fatalf("a deadline kill was classified as a nonzero exit: %v", err)
	}
	if elapsed := time.Since(start); elapsed > promptly {
		t.Fatalf("Run past its deadline returned after %v", elapsed)
	}
}

// Exec has no Stdin field: the child's stdin is empty (os/exec nil Stdin,
// Python stdin=DEVNULL, Rust Stdio::null, Node stdio 'ignore'), so a
// child that reads stdin sees end-of-file at once instead of blocking.
func TestExecChildStdinIsEmpty(t *testing.T) {
	result, err := run(t, evo.Config{}, sh(t.TempDir(), `if read line; then printf 'got:%s' "$line"; else printf eof; fi`))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Stdout != "eof" {
		t.Fatalf("child stdin was not empty: Stdout = %q", result.Stdout)
	}
}

// Evo owns redaction: captured streams in ExecResult pass through the
// Run's Redactor, so a caller inspecting the result never holds the secret.
func TestExecRedactsCapturedStreams(t *testing.T) {
	const secret = "zys1382-secret-token"
	x := evo.Exec{Path: "/bin/sh", Args: []string{"-c", `printf 'out %s\n' "$1"; printf 'err %s\n' "$1" >&2`, "sh", secret}, Dir: t.TempDir()}
	result, err := run(t, evo.Config{Redactor: redactor{secret}}, x)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.Contains(result.Stdout, secret) || strings.Contains(result.Stderr, secret) {
		t.Fatalf("captured streams carry the secret: stdout %q stderr %q", result.Stdout, result.Stderr)
	}
	if !strings.Contains(result.Stdout, "out ") || !strings.Contains(result.Stderr, "err ") {
		t.Fatalf("redaction dropped the surrounding output: stdout %q stderr %q", result.Stdout, result.Stderr)
	}
}

type redactor struct{ secret string }

func (r redactor) RedactString(s string) string { return strings.ReplaceAll(s, r.secret, "[redacted]") }

func TestExecUnderDryRunDoesNotSpawn(t *testing.T) {
	dir := t.TempDir()
	x := sh(dir, "touch ran")
	x.Outputs = evo.Outputs{evo.File{Path: filepath.Join(dir, "never-produced")}}
	result, err := run(t, evo.Config{DryRun: true}, x)
	if err != nil {
		t.Fatalf("dry-run Run = %v (Outputs are not verified when nothing ran)", err)
	}
	if result.Ran {
		t.Fatalf("dry-run result reports Ran")
	}
	if !absent(t, filepath.Join(dir, "ran")) {
		t.Fatalf("dry-run Run spawned the process")
	}
}

// Exec owns capture: the caller wires no writers, yet stdout and stderr
// both reach the run's evidence, which Finish renders.
func TestExecOutputReachesTheRunEvidence(t *testing.T) {
	var rendered syncBuffer
	out := evo.Init(evo.Config{
		Isolated: true, Plain: true, Stdout: &rendered, Stderr: &rendered, StateDir: t.TempDir(),
		Verbosity: evo.VerbosityVerbose,
	})
	task := out.Task("generate").Define(func(ctx context.Context) error {
		_, err := sh(t.TempDir(), "echo zys1382-stdout-line; echo zys1382-stderr-line >&2").Run(ctx)
		return err
	})
	if err := task.Wait(); err != nil {
		t.Fatalf("task: %v", err)
	}
	_ = out.Finish()
	for _, line := range []string{"zys1382-stdout-line", "zys1382-stderr-line"} {
		if !strings.Contains(rendered.String(), line) {
			t.Fatalf("captured %q never reached the run's evidence:\n%s", line, rendered.String())
		}
	}
}
