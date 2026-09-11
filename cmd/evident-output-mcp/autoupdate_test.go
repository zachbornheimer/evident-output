package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAutoUpdate_NewerPinRecordsInstallAndExec(t *testing.T) {
	home := t.TempDir()
	gopath := t.TempDir()
	app := t.TempDir()
	mod := "module app\n\nrequire github.com/zachbornheimer/evident-output v9.9.9\n"
	if err := os.WriteFile(filepath.Join(app, "go.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	env := mapEnv{"HOME": home, "GOPATH": gopath}
	runner := &fakeRunner{}
	execer := &fakeExecer{}
	argv := []string{"evident-output-mcp", "--stdio"}
	maybeAutoUpdate(app, argv, env, osFiles{}, runner, execer, Identity{Version: "0.4.6"})
	if len(runner.runs) == 0 {
		t.Fatal("expected go install for newer pin")
	}
	if runner.runs[0].Name != "go" {
		t.Fatalf("first run %q", runner.runs[0].Name)
	}
	wantPkg := mcpInstallPkg + "@v9.9.9"
	if !containsArg(runner.runs[0].Args, wantPkg) {
		t.Fatalf("install args=%v want %s", runner.runs[0].Args, wantPkg)
	}
	dirPlan, err := PlanInstall("", app, env, osFiles{})
	if err != nil {
		t.Fatal(err)
	}
	if runner.runs[0].Dir != dirPlan.Dir {
		t.Fatalf("auto-update cwd=%q want plan cwd %q", runner.runs[0].Dir, dirPlan.Dir)
	}
	if execer.n != 1 {
		t.Fatalf("exec count=%d want 1", execer.n)
	}
	if execer.argv0 != dirPlan.LinkSrc {
		t.Fatalf("exec argv0=%q want %q", execer.argv0, dirPlan.LinkSrc)
	}
	if len(execer.argv) != len(argv) {
		t.Fatalf("exec argv=%v want original %v", execer.argv, argv)
	}
	for i := range argv {
		if execer.argv[i] != argv[i] {
			t.Fatalf("exec argv=%v want original %v", execer.argv, argv)
		}
	}
}

func TestAutoUpdate_SkipWhenEnvSet(t *testing.T) {
	home := t.TempDir()
	gopath := t.TempDir()
	app := t.TempDir()
	mod := "module app\n\nrequire github.com/zachbornheimer/evident-output v9.9.9\n"
	if err := os.WriteFile(filepath.Join(app, "go.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	env := mapEnv{"HOME": home, "GOPATH": gopath, envNoAutoUpdate: "1"}
	runner := &fakeRunner{}
	execer := &fakeExecer{}
	maybeAutoUpdate(app, []string{"evident-output-mcp"}, env, osFiles{}, runner, execer, Identity{Version: "0.4.6"})
	if len(runner.runs) != 0 || execer.n != 0 {
		t.Fatalf("NO_AUTO_UPDATE must record nothing, runs=%+v exec=%d", runner.runs, execer.n)
	}
}

func TestAutoUpdate_SkipWhenNoGoMod(t *testing.T) {
	env := mapEnv{"HOME": t.TempDir(), "GOPATH": t.TempDir()}
	runner := &fakeRunner{}
	execer := &fakeExecer{}
	maybeAutoUpdate(t.TempDir(), []string{"evident-output-mcp"}, env, osFiles{}, runner, execer, Identity{Version: "0.4.6"})
	if len(runner.runs) != 0 || execer.n != 0 {
		t.Fatalf("missing go.mod must record nothing, runs=%+v exec=%d", runner.runs, execer.n)
	}
}

type fakeExecer struct {
	n     int
	argv0 string
	argv  []string
	env   []string
}

func (f *fakeExecer) Exec(argv0 string, argv, env []string) error {
	f.n++
	f.argv0 = argv0
	f.argv = append([]string(nil), argv...)
	f.env = append([]string(nil), env...)
	return nil
}
