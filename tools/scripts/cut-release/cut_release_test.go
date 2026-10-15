// Package cutrelease holds the release script's tests. The script itself is
// tools/scripts/cut-release.sh; the tests run it against a scratch repository.
package cutrelease

import (
	"archive/zip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	rootModule   = "github.com/zachbornheimer/evident-output/v2"
	mcpModule    = "github.com/zachbornheimer/evident-output/mcp/v2"
	releaseTag   = "v2.0.0"
	mcpReleaseTg = "mcp/" + releaseTag
)

// A published mcp module must not carry a replace directive: `go install
// <mcp>@version` refuses any module whose go.mod has one. The release script
// owns that, so its tagged mcp/go.mod is the evidence.
func TestCutReleaseDropsMCPReplace(t *testing.T) {
	repo := newScratchRepo(t)
	runScript(t, repo)

	mcpGoMod := git(t, repo, "show", mcpReleaseTg+":mcp/go.mod")
	if strings.Contains(mcpGoMod, "replace") || strings.Contains(mcpGoMod, "=>") {
		t.Errorf("%s:mcp/go.mod still holds a replace directive:\n%s", mcpReleaseTg, mcpGoMod)
	}
	if !strings.Contains(mcpGoMod, rootModule+" "+releaseTag) {
		t.Errorf("%s:mcp/go.mod does not require %s %s:\n%s", mcpReleaseTg, rootModule, releaseTag, mcpGoMod)
	}

	rootMCP := git(t, repo, "show", releaseTag+":mcp/go.mod")
	if !strings.Contains(rootMCP, "replace") {
		t.Errorf("root tag %s should still hold the local replace; the mcp tag is the one that drops it", releaseTag)
	}
	rootCommit := git(t, repo, "rev-parse", releaseTag+"^{commit}")
	mcpParent := git(t, repo, "rev-parse", mcpReleaseTg+"^")
	if rootCommit != mcpParent {
		t.Errorf("%s must be tagged one commit after %s (root %s, mcp parent %s)", mcpReleaseTg, releaseTag, rootCommit, mcpParent)
	}
}

func runScript(t *testing.T, repo string) {
	t.Helper()
	proxy := buildRootProxy(t)
	cmd := exec.Command("bash", "tools/scripts/cut-release.sh")
	cmd.Dir = repo
	cmd.Env = append(scratchEnv(t),
		"VERSION="+releaseTag,
		"CUT_RELEASE_PUSH=0",
		"GOPROXY=file://"+proxy,
		"GOSUMDB=off",
		"GOFLAGS=-modcacherw",
		"GOMODCACHE="+t.TempDir(),
		"GOTOOLCHAIN=local",
		"GOWORK=off",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cut-release.sh: %v\n%s", err, out)
	}
}

func scratchEnv(t *testing.T) []string {
	t.Helper()
	return append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
	)
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = scratchEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// newScratchRepo lays down the smallest tree the script's steps run against:
// two Go modules (root and mcp, mcp replaced onto the root), the Release
// constant, a changelog with the in-flight section, and a copy of the script.
func newScratchRepo(t *testing.T) string {
	t.Helper()
	script, err := os.ReadFile(filepath.Join("..", "cut-release.sh"))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"tools/scripts/cut-release.sh":    string(script),
		"go.mod":                          "module " + rootModule + "\n\ngo 1.25.0\n",
		"evo.go":                          "package evo\n",
		"internal/project/wire/stream.go": "package wire\n\nconst Release = \"v1.0.0\"\n",
		"tools/scripts/sync-release-pins/main.go":     "package main\n\nfunc main() {}\n",
		"testdata/api_golden.txt":                     "api\n",
		"CHANGELOG.md":                                "# Changelog\n\n## 2.0.0 (unreleased)\n\n- first\n",
		"mcp/internal/agent/sections/sections.go":     "package sections\n",
		"mcp/go.mod":                                  "module " + mcpModule + "\n\ngo 1.25.0\n\nrequire " + rootModule + " " + releaseTag + "\n\nreplace " + rootModule + " => ..\n",
		"README.md":                                   "# scratch\n",
		"mcp/docs/mcp.md":                             "# mcp\n",
		"mcp/internal/agent/sections/embedded/mcp.md": "# mcp\n",
		"mcp/skills/skill.md":                         "# skill\n",
		"mcp/integrations/integration.md":             "# integration\n",
		"mcp/internal/buildversion/buildversion.go":   "package buildversion\n",
		"mcp/main.go":                                 "package main\n\nimport _ \"" + rootModule + "\"\n\nfunc main() {}\n",
	}
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	git(t, dir, "init", "--quiet", "-b", "main")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "--quiet", "-m", "chore: scratch")
	return dir
}

// buildRootProxy publishes the root module at the release tag into a file://
// GOPROXY, standing in for the public proxy that resolves it after the real
// root tag is pushed.
func buildRootProxy(t *testing.T) string {
	t.Helper()
	proxy := t.TempDir()
	versionDir := filepath.Join(proxy, filepath.FromSlash(rootModule), "@v")
	if err := os.MkdirAll(versionDir, 0o750); err != nil {
		t.Fatal(err)
	}
	goMod := "module " + rootModule + "\n\ngo 1.25.0\n"
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(versionDir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("list", releaseTag+"\n")
	write(releaseTag+".info", `{"Version":"`+releaseTag+`","Time":"2026-01-01T00:00:00Z"}`)
	write(releaseTag+".mod", goMod)

	archive, err := os.Create(filepath.Join(versionDir, releaseTag+".zip"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = archive.Close() })
	zw := zip.NewWriter(archive)
	for name, content := range map[string]string{"go.mod": goMod, "evo.go": "package evo\n"} {
		w, err := zw.Create(rootModule + "@" + releaseTag + "/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return proxy
}
