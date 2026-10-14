package plain

import "unicode/utf8"

// rootColumn is the sibling-column alignment width of root rows
// (fixture-repo-retire-dryrun.md): every root Task declared so far, resolved
// or not. A caller that declares its whole set of siblings before resolving
// any gets the column a batch render would; declaring one, resolving it,
// then declaring the next narrows the column to what was known at each
// commit — the honest limit of rendering immediately (§17.5).
type rootColumn struct{ count, width int }

func (c *rootColumn) add(name string) {
	c.count++
	c.width = max(c.width, utf8.RuneCountInString(name))
}

// nameWidth is the column width, or 0 when fewer than two root Tasks exist
// and there is nothing to align.
func (c rootColumn) nameWidth() int {
	if c.count < 2 {
		return 0
	}
	return c.width
}
