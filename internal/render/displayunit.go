package render

import (
	"fmt"

	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// alreadySatisfiedDetail is spec §19's muted done-row suffix. It is the
// resolution, not a Summary: runDefine leaves Summary empty, and a
// caller-written Done("already satisfied") is a different path that must
// not be how Verify+Define is proven.
const alreadySatisfiedDetail = "already satisfied"

// alreadySatisfiedRowDetail returns the dim §19 suffix when t settled
// ResolutionAlreadySatisfied. Summary does not replace it. Empty when
// this is not that resolution, so callers keep today's Summary behavior.
func alreadySatisfiedRowDetail(t core.TaskSnapshot, color bool) string {
	if t.Resolution != core.ResolutionAlreadySatisfied {
		return ""
	}
	return txt.Dim(alreadySatisfiedDetail, color)
}

// DisplayUnit is evo-rec.md P3's uniform row model: a task row, a Sequence/
// Group header, a fact line, a confirm gate, and a conclusion band
// are the same shape with different slots populated. "A child's elapsed
// time is not shown at the top level" is a slot policy the caller decides
// when it builds the unit — never an omitted switch branch buried in the
// renderer.
type DisplayUnit struct {
	// Glyph is the pre-styled leading glyph (spinner frame, terminal glyph,
	// or a plain-mode state marker) — SGR color already applied by the
	// caller, since color policy varies by renderer (live vs plain).
	Glyph string
	// Name is the row's label, already padded/aligned for this row's
	// column policy (a collection child pads to a fixed width; a
	// standalone row does not).
	Name string
	// Elapsed is the " — Ns" suffix once a row has aged past elapsedAfter
	// (P5), or "" when the row is not old enough, not unresolved, or this
	// row kind never shows one. Kept distinct from Detail so a caller that
	// wants Detail without an already-embedded suffix (e.g. a future JSON
	// or plain-mode-shared unit) can compose them independently, even
	// though today's live-region Render folds Elapsed into Detail's tail.
	Elapsed string
	// Detail is the row's evidentiary payload: a progress bar, a phase,
	// a summary, a byte count, a failure message — whatever this row kind
	// and state combination has to show. Empty means "glyph + name only".
	Detail string
	// Annotations holds nested lines this row owns (e.g. dim warning/fact
	// lines) — reserved for the plain/durable projection's nested "!"
	// lines; the live-region Render below does not consume it (live shows
	// at most one inline annotation, already folded into Detail).
	Annotations []string
}

// Render composes glyph + name + detail into the one line grammar every
// row kind shares: "<indent><glyph> <name>  <detail>", with the trailing
// name padding trimmed when Detail is empty so a bare row never ends in
// dangling whitespace. The glyph-to-name gap is a single space, matching
// the durable/plain projection's WriteTaskAligned (fixture-repo-retire-
// dryrun.md's "✓ branches ...") — mid-run and at-rest rows must agree on
// this typography, not just at final resolution.
func (u DisplayUnit) Render(indent string) string {
	name := u.Name
	if u.Detail == "" {
		return trimTrailingSpace(indent + u.Glyph + " " + name)
	}
	return fmt.Sprintf("%s%s %s  %s", indent, u.Glyph, name, u.Detail)
}

// trimTrailingSpace removes only trailing ASCII spaces — never other
// whitespace — matching the exact strings.TrimRight(nameField, " ") the
// pre-DisplayUnit renderer used for a detail-less row.
func trimTrailingSpace(s string) string {
	end := len(s)
	for end > 0 && s[end-1] == ' ' {
		end--
	}
	return s[:end]
}
