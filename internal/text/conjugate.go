package text

import (
	"strings"
	"unicode"
)

// irregularPastTense holds imperative-verb -> past-tense spellings that the
// default +d/+ed rule gets wrong. Extend this table, not call sites, when a
// new mutation verb needs a special past tense.
var irregularPastTense = map[string]string{
	"write": "wrote",
}

// compoundPastTense holds hyphenated imperative verbs whose past tense does
// not follow the general compound rule below. delete-remote's past tense
// conjugates its first segment and drops the qualifier ("deleted"), but
// fetch-prune's past tense conjugates its second segment instead
// ("pruned") because "prune", not "fetch", is the verb the subject line's
// qualifier leaves to carry meaning. There is no single mechanical rule
// that produces both spellings, so both are named here explicitly; a new
// compound verb defaults to the general rule (first segment conjugated,
// remaining segments dropped) unless it also needs an entry here.
var compoundPastTense = map[string]string{
	"delete-remote": "deleted",
	"fetch-prune":   "pruned",
}

// irregularPlural holds singular -> plural spellings that the default
// +s/+es/+ies rule gets wrong. Extend this table, not call sites, when a new
// object noun needs a special plural.
//
// "package in .venv" is here, not a suffix-rule fix: the default rule
// inflects whichever word ends the phrase, which is correct for the common
// "<adjective> <noun>" shape (English's head noun trails), but wrong once a
// prepositional qualifier follows the countable noun instead — "package in
// .venv" pluralizes to "packages in .venv" (the noun before the
// preposition), not "package in .venvs" (I4's ledger auto-pluralization).
var irregularPlural = map[string]string{
	"child":            "children",
	"tooth":            "teeth",
	"foot":             "feet",
	"leaf":             "leaves",
	"index":            "indices",
	"package in .venv": "packages in .venv",
}

// Pluralize returns the plural spelling of singular when quantity != 1 (an
// irregular table for the common exceptions, else the regular English
// +s/+es/+ies rule), and singular unchanged when quantity == 1 — the
// object-pluralization counterpart to ConjugatePast's verb table, so a
// mutation call site stops writing its own singular/plural noun() switch.
//
// A glob/path/symbol object ("stale origin/*", "*.tmp") renders unchanged at
// any quantity — see isPluralizableWord — instead of blindly gaining a
// trailing "s" ("stale origin/*s") that the +s/+es/+ies rule was never
// designed to produce.
func Pluralize(quantity int64, singular string) string {
	if quantity == 1 || singular == "" {
		return singular
	}
	if plural, ok := irregularPlural[singular]; ok {
		return plural
	}
	if !isPluralizableWord(singular) {
		return singular
	}
	if IsPlural(singular) {
		// Already plural ("local branches", "children", "logs"). Do not emit
		// "brancheses", "childrens", or "logses".
		return singular
	}
	switch {
	case strings.HasSuffix(singular, "y") && !endsInVowelPlusY(singular):
		return singular[:len(singular)-1] + "ies"
	case strings.HasSuffix(singular, "s"), strings.HasSuffix(singular, "x"), strings.HasSuffix(singular, "z"),
		strings.HasSuffix(singular, "ch"), strings.HasSuffix(singular, "sh"):
		return singular + "es"
	default:
		return singular + "s"
	}
}

// singularEndingsThatLookPlural lists the trailing letter pairs that make an
// ordinary singular noun end in "s" — "class", "status", "analysis". Without
// them the "ends in s ⇒ plural" rule would call every one of these plural.
var singularEndingsThatLookPlural = []string{"ss", "us", "is"}

// IsPlural reports whether noun is already the plural spelling of a countable
// English noun — the check behind both halves of the object-plurality
// contract: Pluralize must leave such a word alone at any quantity, and a
// mutation verb must reject one (its object is singular; the ledger
// pluralizes from the quantity). irregularPlural stays the single table: its
// values are the known irregular plurals, and its keys are singulars the
// suffix rules below would otherwise misread.
func IsPlural(noun string) bool {
	if noun == "" {
		return false
	}
	for singular, plural := range irregularPlural {
		if noun == plural {
			return true
		}
		if noun == singular {
			return false
		}
	}
	if !isPluralizableWord(noun) {
		return false
	}
	head := noun
	if i := strings.LastIndex(noun, " "); i >= 0 {
		head = noun[i+1:]
	}
	if !strings.HasSuffix(head, "s") {
		return false
	}
	for _, ending := range singularEndingsThatLookPlural {
		if strings.HasSuffix(head, ending) {
			return false
		}
	}
	return true
}

// isPluralizableWord reports whether singular reads as ordinary English
// words (letters and spaces only) rather than a glob, path, or symbol token
// the English +s/+es/+ies rule would mangle. "stale origin/*" and "*.tmp"
// both fail this check (a path separator, a wildcard, a dot) and render
// unchanged; irregularPlural entries such as "package in .venv" bypass this
// check entirely since the irregular-table lookup runs first.
func isPluralizableWord(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) && r != ' ' {
			return false
		}
	}
	return true
}

func endsInVowelPlusY(s string) bool {
	if len(s) < 2 {
		return false
	}
	switch s[len(s)-2] {
	case 'a', 'e', 'i', 'o', 'u':
		return true
	default:
		return false
	}
}

// ConjugatePast returns the past-tense spelling of an imperative mutation
// verb (delete -> deleted, push -> pushed, write -> wrote) so callers write
// one imperative verb and evo picks the tense for [changed] rows; [planned]
// rows keep the imperative verb as written. Hyphenated compound verbs
// (delete-remote, fetch-prune) fall back to conjugating their first segment
// and dropping the rest, unless compoundPastTense names a specific spelling
// — see that table's comment for why the general rule doesn't always apply.
func ConjugatePast(verb string) string {
	if past, ok := irregularPastTense[verb]; ok {
		return past
	}
	if past, ok := compoundPastTense[verb]; ok {
		return past
	}
	if first, _, ok := strings.Cut(verb, "-"); ok {
		return ConjugatePast(first)
	}
	if strings.HasSuffix(verb, "e") {
		return verb + "d"
	}
	return verb + "ed"
}
