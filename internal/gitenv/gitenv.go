// Package gitenv builds git subprocesses that find their repository from an
// explicit directory and never from the caller's environment.
//
// Git exports GIT_DIR (and friends) to hooks. A child git that inherits them
// operates on the hook's repository no matter its working directory, so a
// test suite run from a pre-push hook could move the pushing repository's
// HEAD and index.
package gitenv

import (
	"strings"

	"github.com/zachbornheimer/evident-output/internal/process"
)

// repositoryLocationVars are the variables that redirect git to a repository
// other than the one its working directory names.
var repositoryLocationVars = map[string]struct{}{
	"GIT_DIR":                          {},
	"GIT_WORK_TREE":                    {},
	"GIT_INDEX_FILE":                   {},
	"GIT_COMMON_DIR":                   {},
	"GIT_OBJECT_DIRECTORY":             {},
	"GIT_ALTERNATE_OBJECT_DIRECTORIES": {},
	"GIT_NAMESPACE":                    {},
	"GIT_PREFIX":                       {},
}

// Scrub returns a copy of env without any repository-location variable.
func Scrub(env []string) []string {
	kept := make([]string, 0, len(env))
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if _, drop := repositoryLocationVars[name]; drop {
			continue
		}
		kept = append(kept, kv)
	}
	return kept
}

// Command returns a git command that resolves its repository from dir. An
// empty dir means the current working directory.
func Command(dir string, args ...string) *process.Cmd {
	cmd := process.NewCmd("git", args...)
	cmd.Dir = dir
	cmd.Env = Scrub(process.Environ())
	return cmd
}
