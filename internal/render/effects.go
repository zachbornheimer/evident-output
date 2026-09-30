package render

import (
	"fmt"
	"strconv"
	"strings"

	txt "github.com/zachbornheimer/evident-output/internal/text"

	"github.com/zachbornheimer/evident-output/internal/core"
)

// ledgerObject renders r.Object pluralized from r.Quantity when the record
// carries a quantity (I4) — an Effect names a singular object
// (evo.EffectSpec{Verb: evo.EffectDelete, Object: "stale local branch",
// Quantity: 2}) and the ledger derives "branches" at render time via
// txt.Pluralize, instead of every call site hand-composing its own
// singular/plural noun with evo.Pluralize. txt.Pluralize itself stays
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
// summed quantities: twelve Effects deleting one "merged branch" each render
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

// EffectSection is one [changed] or [planned] section as the ledger lays
// it out: Kind ("changed" or "planned"), its Subject and records, the verb
// it meant to use when no record survived, the subject column shared with
// sibling sections (NameWidth, 0 = none), and the terminal Width.
type EffectSection struct {
	Kind         string
	Subject      string
	Records      []core.EffectRecord
	IntendedVerb string
	NameWidth    int
	Width        int
}

// WriteEffects renders one ledger section in the layout its records need:
// the honest "nothing to" line when none survived, one collapsed line for
// a single distinct record, and otherwise a header plus bounded rows,
// compact on a narrow terminal and leader-aligned elsewhere.
func WriteEffects(b *strings.Builder, sec EffectSection, s Style) {
	if len(sec.Records) == 0 {
		writeNothingToDo(b, sec)
		return
	}
	tag := s.Paint(fmt.Sprintf("[%s]", sec.Kind), EffectColor(sec.Kind))
	visible := mergeIdenticalEffectRecords(sec.Records)
	if len(visible) == 1 {
		writeEffectLine(b, tag, sec, visible[0])
		return
	}
	fmt.Fprintf(b, "%s  %s\n", tag, sec.Subject)
	omitted := max(len(visible)-maxVisibleEffectRows, 0)
	visible = visible[:len(visible)-omitted]
	if sec.Width > 0 && sec.Width < CompactLayoutMaxWidth {
		writeCompactEffects(b, visible)
	} else {
		writeAlignedEffects(b, visible, s)
	}
	writeEffectOverflow(b, omitted, s)
}

// writeNothingToDo is a section with zero rows: a [planned]/[changed]
// header over nothing would invent a mutation story that never happened
// (evo-rec.md "nothing-to-do" default). The verb is the section's own
// recorded intent, never hand-assembled.
func writeNothingToDo(b *strings.Builder, sec EffectSection) {
	if sec.IntendedVerb != "" {
		fmt.Fprintf(b, "nothing to %s %s\n", sec.IntendedVerb, sec.Subject)
		return
	}
	fmt.Fprintf(b, "nothing to change for %s\n", sec.Subject)
}

// writeEffectLine collapses a section with one distinct record — one
// Effect per task, the common case — to ONE aligned line: "[planned]
// branches   delete 2 local tips" (fixture-repo-retire-dryrun.md). Contract
// §18: the subject pads to the aligned block's width, then exactly two
// spaces before the verb; taskNameColumnMargin does not apply, since no
// inline annotation sits beside it to clear.
func writeEffectLine(b *strings.Builder, tag string, sec EffectSection, r core.EffectRecord) {
	name := txt.PadRight(sec.Subject, sec.NameWidth)
	if r.HasQty {
		fmt.Fprintf(b, "%s %s  %s %d %s\n", tag, name, r.Verb, r.Quantity, ledgerObject(r))
		return
	}
	fmt.Fprintf(b, "%s %s  %s %s\n", tag, name, r.Verb, r.Object)
}

// writeCompactEffects writes rows without leaders for a narrow terminal
// (TXT-016).
func writeCompactEffects(b *strings.Builder, visible []core.EffectRecord) {
	for _, r := range visible {
		if r.HasQty {
			fmt.Fprintf(b, "  %s %d %s\n", r.Verb, r.Quantity, ledgerObject(r))
		} else {
			fmt.Fprintf(b, "  %s %s\n", r.Verb, r.Object)
		}
	}
}

// maxEffectLeader bounds leader fill so a wide verb does not open an
// unbounded gap (TXT-016).
const maxEffectLeader = 12

// writeAlignedEffects writes rows with verbs and quantities in columns,
// joined to a quantity-less object by a dim leader when the gap is wide.
func writeAlignedEffects(b *strings.Builder, visible []core.EffectRecord, s Style) {
	maxVerb, maxQty := 0, 0
	for _, r := range visible {
		maxVerb = max(maxVerb, len(r.Verb))
		if r.HasQty {
			maxQty = max(maxQty, len(strconv.FormatInt(r.Quantity, 10)))
		}
	}
	for _, r := range visible {
		verb := txt.PadRight(r.Verb, maxVerb)
		if r.HasQty {
			qty := txt.PadLeft(strconv.FormatInt(r.Quantity, 10), maxQty)
			fmt.Fprintf(b, "  %s  %s %s\n", verb, qty, ledgerObject(r))
			continue
		}
		if gap := min(maxVerb-len(r.Verb), maxEffectLeader); gap > 2 {
			fmt.Fprintf(b, "  %s%s %s\n", r.Verb, s.Dim(strings.Repeat("·", gap)), r.Object)
			continue
		}
		fmt.Fprintf(b, "  %s  %s %s\n", verb, txt.PadLeft("", maxQty), r.Object)
	}
}

// WriteLedger writes every [changed] then [planned] section of snap, each
// kind aligned to its own subject column.
func WriteLedger(b *strings.Builder, snap core.Snapshot, width int, s Style) {
	changeNameWidth := maxEffectSubjectWidth(snap.Changes, func(c core.ChangesSnapshot) string { return c.Subject })
	for _, ch := range snap.Changes {
		WriteEffects(b, EffectSection{Kind: "changed", Subject: ch.Subject, Records: ch.Records, IntendedVerb: ch.IntendedVerb, NameWidth: changeNameWidth, Width: width}, s)
	}
	planNameWidth := maxEffectSubjectWidth(snap.Plans, func(p core.PlanSnapshot) string { return p.Subject })
	for _, p := range snap.Plans {
		WriteEffects(b, EffectSection{Kind: "planned", Subject: p.Subject, Records: p.Records, IntendedVerb: p.IntendedVerb, NameWidth: planNameWidth, Width: width}, s)
	}
}

// writeEffectOverflow renders the bounded-rows omission line. The overflow
// glyph (txt.Dim "…"/"...") marks it, not "!" — an omitted-count line is a
// viewport limit, not something demanding attention (evo-rec.md "! is
// attention only... Overflow is never !").
func writeEffectOverflow(b *strings.Builder, omitted int, s Style) {
	if omitted <= 0 {
		return
	}
	fmt.Fprintf(b, "  %s  +%d more (not shown)\n", s.OverflowGlyph(), omitted)
}

// WriteAlreadyMutated renders the early-termination "! already mutated: ..."
// line. It fires whenever a run concludes core.Cancelled or core.Failed with at least
// one committed effect — "!" is attention-only (evo-rec.md "Tightened glyph
// vocabulary"), and an empty ledger earns no attention, so the row is
// suppressed entirely rather than rendered as "none". The summary is derived
// mechanically from the Changes ledger, never assembled by the caller
// (evo-rec.md "Taxonomy and mutation lines are derived, never assembled").
func WriteAlreadyMutated(b *strings.Builder, changes []core.ChangesSnapshot, s Style) {
	summary, ok := SummarizeAlreadyMutated(changes)
	if !ok {
		return
	}
	fmt.Fprintf(b, "%s  already mutated: %s\n", s.WarningGlyph(), summary)
}

// SummarizeAlreadyMutated derives the "! already mutated: ..." line's
// content. Sections that committed the same effect are one fragment with one
// count — "1 module created; 1 module created" told the reader nothing twice
// (P7). When more than one distinct effect survives, a fragment a single task
// owns is named, so the reader learns where each effect happened; a run with
// one effect keeps the unqualified spelling. ok is false when nothing
// committed, telling the caller to suppress the row.
func SummarizeAlreadyMutated(changes []core.ChangesSnapshot) (string, bool) {
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
