package core

import "slices"

// The verb spellings.
const (
	Add       = "add"
	Create    = "create"
	Delete    = "delete"
	Install   = "install"
	Push      = "push"
	Remove    = "remove"
	Uninstall = "uninstall"
	Update    = "update"
)

// EffectVerb is one of the closed set of imperative verbs an opaque
// evo.Effect may declare, paired with the evo constant that spells it.
// Defined here (not in internal/engine) so the engine (which defines
// evo.EffectVerb from it) and the review autofixer (which must not link
// the engine) share one list.
type EffectVerb struct {
	Value    string
	Constant string
}

var effectVerbs = []EffectVerb{
	{Add, "EffectAdd"},
	{Create, "EffectCreate"},
	{Delete, "EffectDelete"},
	{Install, "EffectInstall"},
	{Push, "EffectPush"},
	{Remove, "EffectRemove"},
	{Uninstall, "EffectUninstall"},
	{Update, "EffectUpdate"},
}

// EffectVerbs returns every effect verb, in declaration order.
func EffectVerbs() []EffectVerb { return slices.Clone(effectVerbs) }

// EffectVerbConstant is the evo constant spelling value ("EffectDelete"), or
// false when no effect verb is spelled value.
func EffectVerbConstant(value string) (string, bool) {
	i := slices.IndexFunc(effectVerbs, func(v EffectVerb) bool { return v.Value == value })
	if i < 0 {
		return "", false
	}
	return effectVerbs[i].Constant, true
}
