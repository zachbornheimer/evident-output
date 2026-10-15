package plain

import "unicode/utf8"

// rootColumn is the sibling-column alignment width of root rows
// (fixture-repo-retire-dryrun.md): every root Task declared so far, resolved
// or not. A caller that declares its whole set of siblings before resolving
// any gets the column a batch render would; declaring one, resolving it,
// then declaring the next narrows the column to what was known at each
// commit — the honest limit of rendering immediately (§17.5).
//
// A Confirm question is not a sibling: it is a prompt with its own answer, so
// withdraw takes it out once its window opens and a long question never pads
// the rows around it.
type rootColumn struct {
	// widths counts the rows of each name width, so withdrawing the widest
	// row finds the next widest without rescanning the rows.
	widths       map[int]int
	count, width int
}

func (c *rootColumn) add(name string) {
	if c.widths == nil {
		c.widths = map[int]int{}
	}
	w := utf8.RuneCountInString(name)
	c.widths[w]++
	c.count++
	c.width = max(c.width, w)
}

// withdraw takes the row named name back out of the column.
func (c *rootColumn) withdraw(name string) {
	w := utf8.RuneCountInString(name)
	if c.widths[w] == 0 {
		return
	}
	c.count--
	if c.widths[w]--; c.widths[w] > 0 {
		return
	}
	delete(c.widths, w)
	if w == c.width {
		c.width = 0
		for remaining := range c.widths {
			c.width = max(c.width, remaining)
		}
	}
}

// nameWidth is the column width, or 0 when fewer than two root Tasks exist
// and there is nothing to align.
func (c rootColumn) nameWidth() int {
	if c.count < 2 {
		return 0
	}
	return c.width
}
