package core

import "slices"

// The verb spellings.
const (
	EffectVerbAdd       = "add"
	EffectVerbCreate    = "create"
	EffectVerbDelete    = "delete"
	EffectVerbInstall   = "install"
	EffectVerbPush      = "push"
	EffectVerbRemove    = "remove"
	EffectVerbUninstall = "uninstall"
	EffectVerbUpdate    = "update"
)

// EffectVerbSpelling is one of the closed set of imperative verbs an opaque
// evo.Effect may declare, paired with the evo constant that spells it.
// Defined here (not in internal/engine) so the engine (which defines
// evo.EffectVerb from it) and the review autofixer (which must not link
// the engine) share one list.
type EffectVerbSpelling struct {
	Value    string
	Constant string
}

var effectVerbSpellings = []EffectVerbSpelling{
	{EffectVerbAdd, "EffectAdd"},
	{EffectVerbCreate, "EffectCreate"},
	{EffectVerbDelete, "EffectDelete"},
	{EffectVerbInstall, "EffectInstall"},
	{EffectVerbPush, "EffectPush"},
	{EffectVerbRemove, "EffectRemove"},
	{EffectVerbUninstall, "EffectUninstall"},
	{EffectVerbUpdate, "EffectUpdate"},
}

// EffectVerbs returns every effect verb, in declaration order.
func EffectVerbs() []EffectVerbSpelling { return slices.Clone(effectVerbSpellings) }

// EffectVerbConstant is the evo constant spelling value ("EffectDelete"), or
// false when no effect verb is spelled value.
func EffectVerbConstant(value string) (string, bool) {
	i := slices.IndexFunc(effectVerbSpellings, func(v EffectVerbSpelling) bool { return v.Value == value })
	if i < 0 {
		return "", false
	}
	return effectVerbSpellings[i].Constant, true
}
