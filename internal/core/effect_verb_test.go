package core

import "testing"

func TestEveryVerbHasItsConstant(t *testing.T) {
	for _, v := range All() {
		got, ok := Constant(v.Value)
		if !ok || got != v.Constant {
			t.Errorf("Constant(%q) = %q, %v; want %q", v.Value, got, ok, v.Constant)
		}
	}
	if _, ok := Constant("write"); ok {
		t.Error("write is not an EffectVerb: file state goes through evo.File")
	}
}
