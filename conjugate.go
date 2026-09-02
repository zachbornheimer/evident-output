package evo

import "strings"

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

// conjugatePast returns the past-tense spelling of an imperative mutation
// verb (delete -> deleted, push -> pushed, write -> wrote) so callers write
// one imperative verb and evo picks the tense for [changed] rows; [planned]
// rows keep the imperative verb as written. Hyphenated compound verbs
// (delete-remote, fetch-prune) fall back to conjugating their first segment
// and dropping the rest, unless compoundPastTense names a specific spelling
// — see that table's comment for why the general rule doesn't always apply.
func conjugatePast(verb string) string {
	if past, ok := irregularPastTense[verb]; ok {
		return past
	}
	if past, ok := compoundPastTense[verb]; ok {
		return past
	}
	if first, _, ok := strings.Cut(verb, "-"); ok {
		return conjugatePast(first)
	}
	if strings.HasSuffix(verb, "e") {
		return verb + "d"
	}
	return verb + "ed"
}
