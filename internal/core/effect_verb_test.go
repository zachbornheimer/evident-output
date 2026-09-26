package core

import "testing"

func TestEveryVerbHasItsConstant(t *testing.T) {
	for _, v := range EffectVerbs() {
		got, ok := EffectVerbConstant(v.Value)
		if !ok || got != v.Constant {
			t.Errorf("EffectVerbConstant(%q) = %q, %v; want %q", v.Value, got, ok, v.Constant)
		}
	}
	if _, ok := EffectVerbConstant("write"); ok {
		t.Error("write is not an EffectVerb: file state goes through evo.File")
	}
}
