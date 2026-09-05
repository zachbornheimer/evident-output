package main

import (
	"fmt"
	"strings"
)

// renderMarkdown produces the full usage-inventory document for repoRoot's
// files, in the reference shape: a title, an intro sentence naming the
// scanned repo, then one "# <path>" heading per file with one "**via:**"
// + fenced code block per qualifying declaration, in source order.
func renderMarkdown(repoRoot string, files []fileInventory, meta repoMeta) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Evident Output (evo) usage inventory — %s repo\n\n", meta.Name)
	fmt.Fprintf(&b, "Scanned repo: `%s` (%s). Every heading below is a file that uses evo in some "+
		"form; every fenced block is the verbatim enclosing function/type/var declaration for one "+
		"or more usage sites within it.\n", repoRoot, parenClause(meta))

	for _, file := range files {
		fmt.Fprintf(&b, "\n# %s\n", file.Path)
		for _, decl := range file.Decls {
			fmt.Fprintf(&b, "\n**via:** %s\n\n```go\n%s\n```\n", decl.Usage, decl.Source)
		}
	}
	return b.String()
}

// parenClause builds the "(branch `x` @ `y`, evident-output vZ)" segment,
// omitting the branch/SHA half gracefully when repoRoot isn't a git
// worktree.
func parenClause(meta repoMeta) string {
	if !meta.HasBranchAndSHA {
		return meta.VersionClause
	}
	return fmt.Sprintf("branch `%s` @ `%s`, %s", meta.Branch, meta.SHA, meta.VersionClause)
}
