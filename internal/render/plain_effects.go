package render

import (
	"fmt"
	"strconv"
	"strings"

	txt "github.com/zachbornheimer/evident-output/internal/text"

	"github.com/zachbornheimer/evident-output/internal/core"
)

// ledgerObject renders r.Object pluralized from r.Quantity when the record
// carries a quantity (I4) — mutation verbs take a singular object
// (Delete(2, "stale local branch")) and the ledger derives "branches" at
// render time via txt.Pluralize, instead of every call site hand-composing its
// own singular/plural noun with evo.Pluralize. txt.Pluralize itself stays
// exported for prose outside the ledger (a Printf line, a core.Problem detail).
func ledgerObject(r core.EffectRecord) string {
	if !r.HasQty {
		return r.Object
	}
	return txt.Pluralize(r.Quantity, r.Object)
}

// mergeIdenticalEffectRecords combines repeated records that share the same
// (verb, object) pair — one ledger's tense is fixed for every record it
// holds, so verb+object alone identifies a duplicate row — into one row with
// summed quantities: twelve `Delete(1, "merged branch")` calls render
// "deleted 12 merged branches", not twelve identical rows plus an overflow
// ellipsis (release-gate finding 6). Distinct records (different verb or
// object) are left alone and keep their original relative order. The model
// itself is untouched — only this presentation-layer view merges.
func mergeIdenticalEffectRecords(records []core.EffectRecord) []core.EffectRecord {
	type key struct{ verb, object string }
	index := make(map[key]int, len(records))
	merged := make([]core.EffectRecord, 0, len(records))
	for _, r := range records {
		k := key{r.Verb, r.Object}
		if i, ok := index[k]; ok {
			m := &merged[i]
			if !m.HasQty {
				m.HasQty = true
				m.Quantity = 1
			}
			if r.HasQty {
				m.Quantity += r.Quantity
			} else {
				m.Quantity++
			}
			continue
		}
		index[k] = len(merged)
		merged = append(merged, r)
	}
	return merged
}

func WriteEffects(b *strings.Builder, kind, subject string, nameWidth int, records []core.EffectRecord, intendedVerb string, width int, color bool, profile txt.GlyphProfile) {
	// A [planned]/[changed] header with zero rows beneath it invents a mutation
	// story that never happened; render the honest empty-success line instead
	// (evo-rec.md "nothing-to-do" default). The verb comes from the section's
	// own recorded intent — never hand-assembled — falling back to a generic
	// phrasing only when no mutation verb was ever recorded for it.
	if len(records) == 0 {
		if intendedVerb != "" {
			fmt.Fprintf(b, "nothing to %s %s\n", intendedVerb, subject)
		} else {
			fmt.Fprintf(b, "nothing to change for %s\n", subject)
		}
		return
	}
	tag := txt.Style(fmt.Sprintf("[%s]", kind), effectColor(kind), color)

	visible := mergeIdenticalEffectRecords(records)

	// The common case — one mutation call per task — collapses to ONE
	// aligned line per subject: "[planned] branches   delete 2 local tips"
	// (fixture-repo-retire-dryrun.md), instead of a separate header line plus
	// an indented row underneath. A subject with more than one distinct
	// record (rarer — several mutation calls under one task) keeps the
	// original header+rows shape below, since collapsing several records
	// onto one line would lose which quantity/object belongs to which verb.
	if len(visible) == 1 && len(visible) <= maxVisibleEffectRows {
		r := visible[0]
		// Contract §18: pad the subject to the max display width of the
		// aligned block, then exactly two literal spaces before the verb —
		// no taskNameColumnMargin here. That margin exists so a task-row's
		// own inline annotation clears the ledger's leader dots; the
		// collapsed Effect line has no such neighbor to clear, and adding
		// it produced a third space (byte drift from the canonical
		// zq-prune §18 fixture: "[planned] branches         delete ...").
		name := txt.PadRight(subject, nameWidth)
		if r.HasQty {
			fmt.Fprintf(b, "%s %s  %s %d %s\n", tag, name, r.Verb, r.Quantity, ledgerObject(r))
		} else {
			fmt.Fprintf(b, "%s %s  %s %s\n", tag, name, r.Verb, r.Object)
		}
		return
	}

	fmt.Fprintf(b, "%s  %s\n", tag, subject)
	omitted := 0
	if len(visible) > maxVisibleEffectRows {
		omitted = len(visible) - maxVisibleEffectRows
		visible = visible[:maxVisibleEffectRows]
	}

	// TXT-016: leaders omitted when unnecessary (single short column / narrow).
	if width > 0 && width < compactLayoutMaxWidth {
		for _, r := range visible {
			if r.HasQty {
				fmt.Fprintf(b, "  %s %d %s\n", r.Verb, r.Quantity, ledgerObject(r))
			} else {
				fmt.Fprintf(b, "  %s %s\n", r.Verb, r.Object)
			}
		}
		writeEffectOverflow(b, omitted, color, profile)
		return
	}
	maxVerb := 0
	maxQty := 0
	for _, r := range visible {
		if len(r.Verb) > maxVerb {
			maxVerb = len(r.Verb)
		}
		if r.HasQty {
			n := len(strconv.FormatInt(r.Quantity, 10))
			if n > maxQty {
				maxQty = n
			}
		}
	}
	// Bound leader fill so wide verbs do not create unbounded gaps (TXT-016).
	const maxLeader = 12
	for _, r := range visible {
		verb := txt.PadRight(r.Verb, maxVerb)
		if r.HasQty {
			qty := txt.PadLeft(strconv.FormatInt(r.Quantity, 10), maxQty)
			fmt.Fprintf(b, "  %s  %s %s\n", verb, qty, ledgerObject(r))
			continue
		}
		gap := min(maxVerb-len(r.Verb), maxLeader)
		if gap > 2 {
			leader := strings.Repeat("·", gap)
			fmt.Fprintf(b, "  %s%s %s\n", r.Verb, txt.Dim(leader, color), r.Object)
		} else {
			qtyPad := txt.PadLeft("", maxQty)
			fmt.Fprintf(b, "  %s  %s %s\n", verb, qtyPad, r.Object)
		}
	}
	writeEffectOverflow(b, omitted, color, profile)
}

// writeEffectOverflow renders the bounded-rows omission line. The overflow
// glyph (txt.Dim "…"/"...") marks it, not "!" — an omitted-count line is a
// viewport limit, not something demanding attention (evo-rec.md "! is
// attention only... Overflow is never !").
func writeEffectOverflow(b *strings.Builder, omitted int, color bool, profile txt.GlyphProfile) {
	if omitted <= 0 {
		return
	}
	fmt.Fprintf(b, "  %s  +%d more (not shown)\n", txt.Dim(txt.GlyphOverflow.Render(profile), color), omitted)
}

// writeAlreadyMutated renders the early-termination "! already mutated: ..."
// line. It fires whenever a run concludes core.Cancelled or core.Failed with at least
// one committed effect — "!" is attention-only (evo-rec.md "Tightened glyph
// vocabulary"), and an empty ledger earns no attention, so the row is
// suppressed entirely rather than rendered as "none". The summary is derived
// mechanically from the Changes ledger, never assembled by the caller
// (evo-rec.md "Taxonomy and mutation lines are derived, never assembled").
func writeAlreadyMutated(b *strings.Builder, changes []core.ChangesSnapshot, color bool, profile txt.GlyphProfile) {
	summary, ok := summarizeAlreadyMutated(changes)
	if !ok {
		return
	}
	glyph := txt.StyleGlyph(txt.GlyphWarningState.Render(profile), txt.SGRYellow, color)
	fmt.Fprintf(b, "%s  already mutated: %s\n", glyph, summary)
}

// summarizeAlreadyMutated derives the "! already mutated: ..." line's
// content. Sections that committed the same effect are one fragment with one
// count — "1 module created; 1 module created" told the reader nothing twice
// (P7). When more than one distinct effect survives, a fragment a single task
// owns is named, so the reader learns where each effect happened; a run with
// one effect keeps the unqualified spelling. ok is false when nothing
// committed, telling the caller to suppress the row.
func summarizeAlreadyMutated(changes []core.ChangesSnapshot) (string, bool) {
	effects := aggregateMutatedEffects(changes)
	if len(effects) == 0 {
		return "", false
	}
	parts := make([]string, 0, len(effects))
	for _, e := range effects {
		text := e.text()
		if len(effects) > 1 && len(e.owners) == 1 {
			text = e.owners[0] + ": " + text
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, "; "), true
}

// aggregateMutatedEffects reduces every non-empty Changes section to its
// effect and merges the ones that say the same thing, in first-seen order.
func aggregateMutatedEffects(changes []core.ChangesSnapshot) []mutatedEffect {
	var effects []mutatedEffect
	index := map[string]int{}
	for _, ch := range changes {
		if len(ch.Records) == 0 {
			continue
		}
		e := changeSectionEffect(ch)
		key := fmt.Sprintf("%s\x00%s\x00%t", e.verb, e.object, e.countable)
		if at, ok := index[key]; ok {
			effects[at].total += e.total
			effects[at].owners = append(effects[at].owners, e.owners...)
			continue
		}
		index[key] = len(effects)
		effects = append(effects, e)
	}
	return effects
}

// changeSectionEffect sums a section's record quantities (no-qty records
// count as 1 each) and reports the shared verb/object when every record
// agrees, falling back to the section's subject when records name distinct
// verbs or objects.
func changeSectionEffect(ch core.ChangesSnapshot) mutatedEffect {
	var total int64
	verb, mixedVerb := ch.Records[0].Verb, false
	object, mixedObject := ch.Records[0].Object, false
	for _, r := range ch.Records {
		if r.HasQty {
			total += r.Quantity
		} else {
			total++
		}
		if r.Verb != verb {
			mixedVerb = true
		}
		if r.Object != object {
			mixedObject = true
		}
	}
	e := mutatedEffect{verb: verb, object: object, countable: true, total: total, owners: []string{ch.Subject}}
	if mixedObject {
		e.object, e.countable = ch.Subject, false
	}
	if mixedVerb {
		e.verb = ""
	}
	return e
}

// mutatedEffect is what one committed effect states on the early-termination
// line: how many, of what, done how — and which tasks did it.
type mutatedEffect struct {
	verb string
	// object is singular and pluralized from total at render time, unless
	// countable is false — a section whose records name distinct objects
	// falls back to the section's own subject, which is a name, not a noun.
	object    string
	countable bool
	total     int64
	owners    []string
}

func (e mutatedEffect) text() string {
	object := e.object
	if e.countable {
		object = txt.Pluralize(e.total, object)
	}
	if e.verb == "" {
		return fmt.Sprintf("%d %s changed", e.total, object)
	}
	return fmt.Sprintf("%d %s %s", e.total, object, e.verb)
}
