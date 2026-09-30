package core

// The closed set of imperative verbs an opaque evo.Effect may declare. The
// engine (which defines evo.EffectVerb from it) and the review autofixer
// (which must not link the engine) share this one list.

import "slices"

// The EffectVerb spellings.
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

// EffectVerbSpelling is one verb and the evo constant that spells it.
type EffectVerbSpelling struct {
	Value    string
	Constant string
}

var all = []EffectVerbSpelling{
	{EffectVerbAdd, "EffectAdd"},
	{EffectVerbCreate, "EffectCreate"},
	{EffectVerbDelete, "EffectDelete"},
	{EffectVerbInstall, "EffectInstall"},
	{EffectVerbPush, "EffectPush"},
	{EffectVerbRemove, "EffectRemove"},
	{EffectVerbUninstall, "EffectUninstall"},
	{EffectVerbUpdate, "EffectUpdate"},
}

// EffectVerbs returns every verb, in declaration order.
func EffectVerbs() []EffectVerbSpelling { return slices.Clone(all) }

// EffectVerbConstant is the evo constant spelling value ("EffectDelete"), or false
// when no verb is spelled value.
func EffectVerbConstant(value string) (string, bool) {
	i := slices.IndexFunc(all, func(v EffectVerbSpelling) bool { return v.Value == value })
	if i < 0 {
		return "", false
	}
	return all[i].Constant, true
}
