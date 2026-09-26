// EffectVerbs is the closed set of imperative verbs an opaque evo.Effect
// may declare, defined here (not in internal/engine) so the engine (which
// defines evo.EffectVerb from it) and the review autofixer (which must not
// link the engine) share one list.
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

// Verb is one verb and the evo constant that spells it.
type Verb struct {
	Value    string
	Constant string
}

var all = []Verb{
	{Add, "EffectAdd"},
	{Create, "EffectCreate"},
	{Delete, "EffectDelete"},
	{Install, "EffectInstall"},
	{Push, "EffectPush"},
	{Remove, "EffectRemove"},
	{Uninstall, "EffectUninstall"},
	{Update, "EffectUpdate"},
}

// All returns every verb, in declaration order.
func All() []Verb { return slices.Clone(all) }

// Constant is the evo constant spelling value ("EffectDelete"), or false
// when no verb is spelled value.
func Constant(value string) (string, bool) {
	i := slices.IndexFunc(all, func(v Verb) bool { return v.Value == value })
	if i < 0 {
		return "", false
	}
	return all[i].Constant, true
}
