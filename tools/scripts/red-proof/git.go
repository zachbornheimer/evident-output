package main

import (
	"fmt"
	"regexp"
)

var hexCommit = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

// checkRev accepts HEAD or a hex commit id, so no ledger text reaches git as an option or expression.
func checkRev(rev string) error {
	if rev != headRev && !hexCommit.MatchString(rev) {
		return fmt.Errorf("revision %q is neither HEAD nor a hex commit id", rev)
	}
	return nil
}

type repoGit struct{ root string }

func (g repoGit) Export(rev, dir string) error {
	return gitArchiveInto(g.root, rev, dir)
}

// ChangedFiles lists the files commit rev changed; available is false when the
// repository does not hold the commit.
func (g repoGit) ChangedFiles(rev string) (files []string, available bool, err error) {
	return gitCommitFiles(g.root, rev)
}

// BlameOrigins names the path each of the given lines of a HEAD file came from.
func (g repoGit) BlameOrigins(file string, lines []int) ([]string, error) {
	return gitBlameOrigins(g.root, file, lines)
}

// HeadFile reads a file as committed at HEAD.
func (g repoGit) HeadFile(file string) ([]byte, error) {
	return gitShowHead(g.root, file)
}
