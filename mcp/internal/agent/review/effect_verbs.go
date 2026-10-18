package review

import "slices"

// effectVerbSpelling is one opaque-Effect verb spelling a retired call site
// can carry and the exported evo.Verb that replaces it. The contract's Verb
// is the closed set Create, Update and Delete, so verbs that spell the same
// mutation (add, install, create) collapse onto one. The list is owned here
// because the review autofixer must not link the engine;
// internal/change's TestEffectVerbSpellingsMatchReviewList fails if its
// spellings drift from the engine's own.
type effectVerbSpelling struct {
	Value    string
	Exported string
}

var effectVerbSpellings = []effectVerbSpelling{
	{"add", "Create"},
	{"create", "Create"},
	{"delete", "Delete"},
	{"install", "Create"},
	{"push", "Update"},
	{"remove", "Delete"},
	{"uninstall", "Delete"},
	{"update", "Update"},
}

// exportedVerbNames are the evo.Verb constants the contract exports.
var exportedVerbNames = []string{"Create", "Update", "Delete"}

// exportedVerb is the evo.Verb constant that replaces the verb spelled
// value, or false when no verb is spelled value.
func exportedVerb(value string) (string, bool) {
	i := slices.IndexFunc(effectVerbSpellings, func(v effectVerbSpelling) bool { return v.Value == value })
	if i < 0 {
		return "", false
	}
	return effectVerbSpellings[i].Exported, true
}
