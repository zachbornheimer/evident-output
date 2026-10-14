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
