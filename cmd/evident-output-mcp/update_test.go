package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/modpin"
)

func TestUpdateCLI_NeitherVersionNorDirectoryErrors(t *testing.T) {
	stderr, code := captureUpdate(t, []string{"update"})
	if code != 2 {
		t.Fatalf("exit %d, want 2; stderr=%q", code, stderr)
	}
	if !strings.Contains(stderr, "--version") || !strings.Contains(stderr, "--directory") {
		t.Fatalf("error must require --version or --directory, got %q", stderr)
	}
}

func TestUpdateCLI_BothFlagsError(t *testing.T) {
	stderr, code := captureUpdate(t, []string{"update", "--version", "v0.4.6", "--directory", t.TempDir()})
	if code != 2 {
		t.Fatalf("exit %d, want 2; stderr=%q", code, stderr)
	}
}

func TestUpdateCLI_NotUpdateReturnsUnhandled(t *testing.T) {
	if code := runUpdate([]string{"config", "--client", "grok"}); code != -1 {
		t.Fatalf("runUpdate must leave non-update argv unhandled, got %d", code)
	}
}

func TestPlanInstall_ReplacePathUsesTreeAsCwd(t *testing.T) {
	home := t.TempDir()
	gopath := t.TempDir()
	app := t.TempDir()
	evoTree := filepath.Join(app, "evo")
	if err := os.Mkdir(evoTree, 0o755); err != nil {
		t.Fatal(err)
	}
	mod := "module app\n\nrequire github.com/zachbornheimer/evident-output v0.4.6\nreplace github.com/zachbornheimer/evident-output => ./evo\n"
	if err := os.WriteFile(filepath.Join(app, "go.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanInstall("", app, mapEnv{"HOME": home, "GOPATH": gopath}, osFiles{})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Dir != evoTree {
		t.Fatalf("cwd=%q want replace tree %q", plan.Dir, evoTree)
	}
	if !containsArg(plan.GoArgs, mcpLocalPkg) {
		t.Fatalf("pkg=%v want %s", plan.GoArgs, mcpLocalPkg)
	}
	if !containsArg(plan.GoArgs, "-C") || !containsArg(plan.GoArgs, evoTree) {
		t.Fatalf("want go install -C %s, got %v", evoTree, plan.GoArgs)
	}
	assertNoCopyArgs(t, plan.GoArgs)
}

func TestPlanInstall_SelfModuleUsesLocalTree(t *testing.T) {
	home := t.TempDir()
	gopath := t.TempDir()
	tree := t.TempDir()
	mod := "module github.com/zachbornheimer/evident-output\n\ngo 1.25.0\n"
	if err := os.WriteFile(filepath.Join(tree, "go.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanInstall("", tree, mapEnv{"HOME": home, "GOPATH": gopath}, osFiles{})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Dir != tree {
		t.Fatalf("cwd=%q want module dir %q", plan.Dir, tree)
	}
	if !containsArg(plan.GoArgs, mcpLocalPkg) {
		t.Fatalf("self-module must install %s, got %v", mcpLocalPkg, plan.GoArgs)
	}
}

func TestPlanInstall_SemverPin(t *testing.T) {
	home := t.TempDir()
	gopath := t.TempDir()
	plan, err := PlanInstall("v0.4.6", "", mapEnv{"HOME": home, "GOPATH": gopath}, osFiles{})
	if err != nil {
		t.Fatal(err)
	}
	wantPkg := mcpInstallPkg + "@v0.4.6"
	if !containsArg(plan.GoArgs, wantPkg) {
		t.Fatalf("got %v, want pkg %s", plan.GoArgs, wantPkg)
	}
	if containsArg(plan.GoArgs, "-C") {
		t.Fatalf("semver pin must not use -C, got %v", plan.GoArgs)
	}
}

func TestExecuteInstall_GopathBinThenSymlink(t *testing.T) {
	home := t.TempDir()
	gopath := t.TempDir()
	env := mapEnv{"HOME": home, "GOPATH": gopath}
	plan, err := PlanInstall("v0.4.6", "", env, osFiles{})
	if err != nil {
		t.Fatal(err)
	}
	installDir := filepath.Join(gopath, "bin")
	if plan.GOBIN != installDir {
		t.Fatalf("GOBIN=%q want %q", plan.GOBIN, installDir)
	}
	linkDest := filepath.Join(home, localBinRelHome, mcpBinaryName)
	if plan.LinkDest != linkDest {
		t.Fatalf("LinkDest=%q want %q", plan.LinkDest, linkDest)
	}
	if plan.SkipLink {
		t.Fatal("expected symlink when GOPATH/bin != ~/.local/bin")
	}
	runner := &fakeRunner{}
	if err := ExecuteInstall(plan, runner, osFiles{}); err != nil {
		t.Fatal(err)
	}
	if len(runner.runs) != 2 {
		t.Fatalf("runs=%d want 2 (go install + ln), got %+v", len(runner.runs), runner.runs)
	}
	goRun := runner.runs[0]
	if goRun.Name != "go" {
		t.Fatalf("first command %q", goRun.Name)
	}
	if goRun.Dir != plan.Dir {
		t.Fatalf("go cwd=%q want %q", goRun.Dir, plan.Dir)
	}
	if !containsArg(goRun.Env, "GOBIN="+installDir) {
		t.Fatalf("GOBIN env=%v", goRun.Env)
	}
	assertNoCopyArgs(t, goRun.Args)
	lnRun := runner.runs[1]
	if lnRun.Name != "ln" {
		t.Fatalf("second command %q want ln", lnRun.Name)
	}
	if !containsArg(lnRun.Args, "-sfn") {
		t.Fatalf("ln args=%v want -sfn", lnRun.Args)
	}
	if !containsArg(lnRun.Args, plan.LinkSrc) || !containsArg(lnRun.Args, plan.LinkDest) {
		t.Fatalf("ln args=%v want %s -> %s", lnRun.Args, plan.LinkSrc, plan.LinkDest)
	}
}

func TestExecuteInstall_SkipLnWhenSrcEqualsDest(t *testing.T) {
	home := t.TempDir()
	gopath := filepath.Join(home, ".local")
	env := mapEnv{"HOME": home, "GOPATH": gopath}
	plan, err := PlanInstall("v0.4.6", "", env, osFiles{})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.SkipLink {
		t.Fatalf("expected SkipLink when installDir==linkDest; GOBIN=%s LinkDest=%s", plan.GOBIN, filepath.Dir(plan.LinkDest))
	}
	runner := &fakeRunner{}
	if err := ExecuteInstall(plan, runner, osFiles{}); err != nil {
		t.Fatal(err)
	}
	for _, r := range runner.runs {
		if r.Name == "ln" {
			t.Fatalf("must not ln when src==dest: %+v", runner.runs)
		}
		assertNoCopyArgs(t, r.Args)
	}
	if len(runner.runs) != 1 || runner.runs[0].Name != "go" {
		t.Fatalf("want only go install, got %+v", runner.runs)
	}
}

func TestPlanInstall_IgnoresGOBINWhenItIsLocalBin(t *testing.T) {
	home := t.TempDir()
	gopath := t.TempDir()
	localBin := filepath.Join(home, localBinRelHome)
	env := mapEnv{"HOME": home, "GOPATH": gopath, "GOBIN": localBin}
	plan, err := PlanInstall("v0.4.6", "", env, osFiles{})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(gopath, "bin")
	if plan.GOBIN != want {
		t.Fatalf("GOBIN=%q want GOPATH/bin %q (ignored ~/.local/bin)", plan.GOBIN, want)
	}
}

func TestPlanInstall_NonPathReplaceErrors(t *testing.T) {
	home := t.TempDir()
	dir := t.TempDir()
	mod := "module app\nreplace github.com/zachbornheimer/evident-output => github.com/other/evident-output v1.2.3\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := PlanInstall("", dir, mapEnv{"HOME": home, "GOPATH": t.TempDir()}, osFiles{})
	if err == nil || !strings.Contains(err.Error(), "path") {
		t.Fatalf("got %v, want non-path replace error", err)
	}
}

func TestUpdateNeeded_FalseWhenServerNewerThanPin(t *testing.T) {
	needed := updateNeeded(Identity{Version: "9.9.9"}, modpin.Pin{Version: "v0.4.6"})
	if needed {
		t.Fatal("server newer than pin must not need update")
	}
}

func TestUpdateNeeded_TrueWhenReplaceAndNotBuiltFromPath(t *testing.T) {
	needed := updateNeeded(Identity{Version: "0.4.6", SourceDir: "/other"}, modpin.Pin{
		Version:     "v0.4.6",
		ReplacePath: "/src/evo",
	})
	if !needed {
		t.Fatal("path replace with different build dir must need update")
	}
}

func TestUpdateNeeded_FalseWhenBuiltFromReplacePath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "evo")
	needed := updateNeeded(Identity{Version: "dev", SourceDir: dir}, modpin.Pin{ReplacePath: dir})
	if needed {
		t.Fatal("binary built from replace path must not need update")
	}
}

func TestUpdateNeeded_FalseWhenVersionIsDevOnlyDifference(t *testing.T) {
	needed := updateNeeded(Identity{Version: "dev"}, modpin.Pin{Version: "v0.4.6"})
	if needed {
		t.Fatal("Version=dev is not enough to need update")
	}
}

func TestUpdateCLI_BinaryNeitherFlagDoesNotServe(t *testing.T) {
	bin := buildMCP(t)
	for i := 0; i < 2; i++ {
		cmd := exec.Command(bin, "update")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		if err == nil {
			t.Fatalf("run %d: expected non-zero exit", i+1)
		}
		combined := stdout.String() + stderr.String()
		if strings.Contains(combined, "starting (stdio)") {
			t.Fatalf("run %d: must not start stdio serve: %q", i+1, combined)
		}
		if !strings.Contains(stderr.String(), "--version") || !strings.Contains(stderr.String(), "--directory") {
			t.Fatalf("run %d: stderr=%q", i+1, stderr.String())
		}
	}
}

func captureUpdate(t *testing.T, args []string) (stderr string, code int) {
	t.Helper()
	var buf bytes.Buffer
	prev := updateErr
	updateErr = &buf
	defer func() { updateErr = prev }()
	code = runUpdate(args)
	return buf.String(), code
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func assertNoCopyArgs(t *testing.T, args []string) {
	t.Helper()
	for _, a := range args {
		if a == "cp" || a == "copy" || strings.Contains(a, "Copy") || strings.Contains(a, "WriteFile") {
			t.Fatalf("install must not copy the binary, args=%v", args)
		}
	}
}

type fakeRunner struct {
	runs []recordedRun
}

type recordedRun struct {
	Name string
	Args []string
	Dir  string
	Env  []string
}

func (f *fakeRunner) Run(c Command) error {
	f.runs = append(f.runs, recordedRun(c))
	return nil
}

type mapEnv map[string]string

func (e mapEnv) Get(key string) string { return e[key] }
