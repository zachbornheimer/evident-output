package review

import "slices"

// effectVerbSpelling is one opaque-Effect verb and the evo constant that
// spells it. The list is owned here because the review autofixer must not
// link the engine; internal/change's TestEffectVerbSpellingsMatchReviewList
// fails if it drifts from the engine's own.
type effectVerbSpelling struct {
	Value    string
	Constant string
}

var effectVerbSpellings = []effectVerbSpelling{
	{"add", "EffectAdd"},
	{"create", "EffectCreate"},
	{"delete", "EffectDelete"},
	{"install", "EffectInstall"},
	{"push", "EffectPush"},
	{"remove", "EffectRemove"},
	{"uninstall", "EffectUninstall"},
	{"update", "EffectUpdate"},
}

// effectVerbConstant is the evo constant spelling value, or false when no
// verb is spelled value.
func effectVerbConstant(value string) (string, bool) {
	i := slices.IndexFunc(effectVerbSpellings, func(v effectVerbSpelling) bool { return v.Value == value })
	if i < 0 {
		return "", false
	}
	return effectVerbSpellings[i].Constant, true
}
