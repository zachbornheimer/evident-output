// This file owns the closed set of imperative verbs an opaque Effect may
// declare. There is deliberately no write verb: file state goes through File
// (and Patch), never an opaque callback. The review autofixer shares this one
// list, so it never has to link the engine.

package change

import "slices"

// EffectVerb is the closed set of imperative verbs an opaque Effect may
// declare.
type EffectVerb string

// The complete EffectVerb enum.
const (
	EffectAdd       EffectVerb = "add"
	EffectCreate    EffectVerb = "create"
	EffectDelete    EffectVerb = "delete"
	EffectInstall   EffectVerb = "install"
	EffectPush      EffectVerb = "push"
	EffectRemove    EffectVerb = "remove"
	EffectUninstall EffectVerb = "uninstall"
	EffectUpdate    EffectVerb = "update"
)

// EffectVerbSpelling is one verb and the evo constant that spells it.
type EffectVerbSpelling struct {
	Value    string
	Constant string
}

var effectVerbSpellings = []EffectVerbSpelling{
	{string(EffectAdd), "EffectAdd"},
	{string(EffectCreate), "EffectCreate"},
	{string(EffectDelete), "EffectDelete"},
	{string(EffectInstall), "EffectInstall"},
	{string(EffectPush), "EffectPush"},
	{string(EffectRemove), "EffectRemove"},
	{string(EffectUninstall), "EffectUninstall"},
	{string(EffectUpdate), "EffectUpdate"},
}

// EffectVerbs returns every verb, in declaration order.
func EffectVerbs() []EffectVerbSpelling { return slices.Clone(effectVerbSpellings) }

// EffectVerbConstant is the evo constant spelling value ("EffectDelete"), or
// false when no verb is spelled value.
func EffectVerbConstant(value string) (string, bool) {
	i := slices.IndexFunc(effectVerbSpellings, func(v EffectVerbSpelling) bool { return v.Value == value })
	if i < 0 {
		return "", false
	}
	return effectVerbSpellings[i].Constant, true
}

// valid reports whether v is one of the declared EffectVerb constants.
func (v EffectVerb) valid() bool {
	_, ok := EffectVerbConstant(string(v))
	return ok
}
