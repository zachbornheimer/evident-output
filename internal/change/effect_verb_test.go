package change

import (
	"bytes"
	"fmt"
	"os"
	"testing"
)

// reviewEffectVerbsFile is the review autofixer's own copy of the verb list.
const reviewEffectVerbsFile = "../../mcp/internal/agent/review/effect_verbs.go"

func TestEveryVerbHasItsEffectVerbConstant(t *testing.T) {
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

func TestEffectVerbSpellingsMatchReviewList(t *testing.T) {
	body, err := os.ReadFile(reviewEffectVerbsFile)
	if err != nil {
		t.Fatalf("read %s: %v", reviewEffectVerbsFile, err)
	}
	for _, v := range EffectVerbs() {
		entry := fmt.Sprintf("{%q, %q},", v.Value, v.Constant)
		if !bytes.Contains(body, []byte(entry)) {
			t.Errorf("%s lacks %s", reviewEffectVerbsFile, entry)
		}
	}
	if got, want := bytes.Count(body, []byte("{\"")), len(EffectVerbs()); got != want {
		t.Errorf("%s lists %d verbs, engine declares %d", reviewEffectVerbsFile, got, want)
	}
}
