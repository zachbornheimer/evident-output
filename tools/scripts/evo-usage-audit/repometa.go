package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// repoMeta is the intro-line context for one scanned repo.
type repoMeta struct {
	Name            string // repo directory basename
	VersionClause   string // "evident-output vX.Y.Z" or "evident-output not in go.mod"
	Branch, SHA     string
	HasBranchAndSHA bool
}

// gatherRepoMeta reads root's go.mod for the evident-output require line
// and, if root is a git worktree, its branch and short SHA.
func gatherRepoMeta(root string) repoMeta {
	meta := repoMeta{Name: filepath.Base(root), VersionClause: versionClause(root)}
	if branch, sha, ok := gitBranchAndSHA(root); ok {
		meta.Branch, meta.SHA, meta.HasBranchAndSHA = branch, sha, true
	}
	return meta
}

func versionClause(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "evident-output not in go.mod"
	}
	ver := requireVersion(string(data), evoModulePath)
	if ver == "" {
		return "evident-output not in go.mod"
	}
	return "evident-output " + ver
}

// requireVersion finds modulePath's version in go.mod source, honoring both
// the single-line ("require path v1.2.3") and block ("require (\n\tpath
// v1.2.3\n)") forms. It returns "" when modulePath is not required.
func requireVersion(goMod, modulePath string) string {
	inBlock := false
	for _, line := range strings.Split(goMod, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "require ("):
			inBlock = true
			continue
		case inBlock && trimmed == ")":
			inBlock = false
			continue
		}

		var rest string
		switch {
		case inBlock:
			rest = trimmed
		case strings.HasPrefix(trimmed, "require "):
			rest = strings.TrimPrefix(trimmed, "require ")
		default:
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) >= 2 && fields[0] == modulePath {
			return fields[1]
		}
	}
	return ""
}

// gitBranchAndSHA reports root's current branch and short commit SHA via
// git itself; ok is false when root is not inside a git worktree (or git is
// unavailable), so callers can omit the clause gracefully.
func gitBranchAndSHA(root string) (branch, sha string, ok bool) {
	branch, err := runGit(root, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", "", false
	}
	sha, err = runGit(root, "rev-parse", "--short", "HEAD")
	if err != nil {
		return "", "", false
	}
	return branch, sha, true
}

// runGit executes git rooted at dir, discovering its repository purely from
// dir rather than any ambient GIT_DIR/GIT_WORK_TREE — a caller running
// inside a git hook (e.g. this tool's own pre-push) has those set in its
// environment, and without stripping them a scanned dir with no .git of its
// own would silently resolve to the hook's repository instead.
func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = withoutGitDirEnv(os.Environ())
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git -C %s %s: %w", dir, strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

func withoutGitDirEnv(env []string) []string {
	kept := env[:0]
	for _, kv := range env {
		if strings.HasPrefix(kv, "GIT_DIR=") || strings.HasPrefix(kv, "GIT_WORK_TREE=") {
			continue
		}
		kept = append(kept, kv)
	}
	return kept
}
