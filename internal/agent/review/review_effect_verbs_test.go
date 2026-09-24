package review

import (
	"slices"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/engine"
)

// TestEffectVerbsMatchEngine keeps API-061's Record* rewrite in step with
// the engine's closed EffectVerb set: a new verb must be suggestable.
func TestEffectVerbsMatchEngine(t *testing.T) {
	var want []string
	for _, v := range engine.EffectVerbs() {
		want = append(want, string(v))
	}
	if !slices.Equal(effectVerbs, want) {
		t.Fatalf("review effectVerbs = %v, engine.EffectVerbs() = %v", effectVerbs, want)
	}
	for _, v := range engine.EffectVerbs() {
		if _, ok := effectVerbConstant(string(v)); !ok {
			t.Errorf("no constant spelling for %q", v)
		}
	}
}
