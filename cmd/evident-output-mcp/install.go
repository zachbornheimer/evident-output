package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zachbornheimer/evident-output/internal/modpin"
)

// InstallPlan is the go install + symlink the update runner executes.
type InstallPlan struct {
	GoArgs    []string
	Dir       string
	GOBIN     string
	LinkSrc   string
	LinkDest  string
	SkipLink  bool
	Pin       modpin.Pin
	BuiltFrom string
}

func PlanInstall(version, directory string, env Env, files Files) (InstallPlan, error) {
	hasVer := strings.TrimSpace(version) != ""
	hasDir := strings.TrimSpace(directory) != ""
	if hasVer == hasDir {
		return InstallPlan{}, fmt.Errorf("update requires --version XOR --directory")
	}
	installDir := resolveInstallDir(env)
	linkDir := filepath.Join(env.Get("HOME"), localBinRelHome)
	linkDest := filepath.Join(linkDir, mcpBinaryName)
	linkSrc := filepath.Join(installDir, mcpBinaryName)

	plan := InstallPlan{
		GOBIN:    installDir,
		LinkSrc:  linkSrc,
		LinkDest: linkDest,
		SkipLink: filepath.Clean(linkSrc) == filepath.Clean(linkDest),
	}

	if hasVer {
		pin := strings.TrimSpace(version)
		if !strings.HasPrefix(pin, "v") {
			pin = "v" + pin
		}
		plan.GoArgs = []string{"install", mcpInstallPkg + "@" + pin}
		plan.Pin = modpin.Pin{Version: pin}
		return plan, nil
	}

	modDir, pin, err := pinFromWalk(directory, files)
	if err != nil {
		return InstallPlan{}, err
	}
	plan.Pin = pin
	switch {
	case pin.ReplacePath != "":
		plan.Dir = pin.ReplacePath
		plan.GoArgs = []string{"install", "-C", pin.ReplacePath, mcpLocalPkg}
		plan.BuiltFrom = pin.ReplacePath
	case pin.SelfModule:
		plan.Dir = modDir
		plan.GoArgs = []string{"install", "-C", modDir, mcpLocalPkg}
		plan.BuiltFrom = modDir
	case pin.Version != "":
		plan.GoArgs = []string{"install", mcpInstallPkg + "@" + pin.Version}
	default:
		return InstallPlan{}, fmt.Errorf("go.mod at %s has no evident-output pin", modDir)
	}
	return plan, nil
}

func ExecuteInstall(plan InstallPlan, run Runner, files Files) error {
	if err := files.MkdirAll(filepath.Dir(plan.LinkDest), 0o755); err != nil {
		return fmt.Errorf("mkdir link dest: %w", err)
	}
	if err := run.Run(Command{
		Name: "go",
		Args: plan.GoArgs,
		Dir:  plan.Dir,
		Env:  []string{"GOBIN=" + plan.GOBIN},
	}); err != nil {
		return fmt.Errorf("go install: %w", err)
	}
	if plan.SkipLink {
		return nil
	}
	if err := run.Run(Command{
		Name: "ln",
		Args: []string{"-sfn", plan.LinkSrc, plan.LinkDest},
	}); err != nil {
		return fmt.Errorf("ln -sfn: %w", err)
	}
	return nil
}

func resolveInstallDir(env Env) string {
	home := env.Get("HOME")
	linkDir := filepath.Join(home, localBinRelHome)
	if gobin := env.Get("GOBIN"); gobin != "" && filepath.Clean(gobin) != filepath.Clean(linkDir) {
		return gobin
	}
	gopath := env.Get("GOPATH")
	if gopath == "" {
		gopath = filepath.Join(home, "go")
	}
	return filepath.Join(gopath, "bin")
}

func pinFromWalk(start string, files Files) (string, modpin.Pin, error) {
	dir := start
	if !filepath.IsAbs(dir) {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return "", modpin.Pin{}, fmt.Errorf("directory %q: %w", start, err)
		}
		dir = abs
	}
	for {
		path := filepath.Join(dir, "go.mod")
		data, err := files.ReadFile(path)
		if err == nil {
			pin, err := modpin.Parse(string(data), dir)
			if err != nil {
				return "", modpin.Pin{}, fmt.Errorf("parse %s: %w", path, err)
			}
			return dir, pin, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", modpin.Pin{}, fmt.Errorf("no go.mod above %s", start)
		}
		dir = parent
	}
}
