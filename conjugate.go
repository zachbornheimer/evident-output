package evo

import "strings"

// irregularPastTense holds imperative-verb -> past-tense spellings that the
// default +d/+ed rule gets wrong. Extend this table, not call sites, when a
// new mutation verb needs a special past tense.
var irregularPastTense = map[string]string{
	"write": "wrote",
}

// conjugatePast returns the past-tense spelling of an imperative mutation
// verb (delete -> deleted, push -> pushed, write -> wrote) so callers write
// one imperative verb and evo picks the tense for [changed] rows; [planned]
// rows keep the imperative verb as written.
func conjugatePast(verb string) string {
	if past, ok := irregularPastTense[verb]; ok {
		return past
	}
	if strings.HasSuffix(verb, "e") {
		return verb + "d"
	}
	return verb + "ed"
}
