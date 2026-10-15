package wireschema_test

import (
	"testing"

	"github.com/zachbornheimer/evident-output/v2/testkit/wireschema"
)

// TestValidate_FollowsStandardTypeSemantics pins that the validator reads a schema as a stock draft 2020-12
// validator does: JSON null is a type of its own, and a type list accepts exactly the kinds it names.
func TestValidate_FollowsStandardTypeSemantics(t *testing.T) {
	cases := []struct {
		name, schema, doc string
		valid             bool
	}{
		{"null is not a string", `{"type":"string"}`, `null`, false},
		{"null is not an object", `{"type":"object"}`, `null`, false},
		{"null fails a required string field", `{"type":"object","properties":{"a":{"type":"string"}}}`, `{"a":null}`, false},
		{"null is a null", `{"type":"null"}`, `null`, true},
		{"a type list accepts null", `{"type":["string","null"]}`, `null`, true},
		{"a type list accepts its other kind", `{"type":["string","null"]}`, `"x"`, true},
		{"a type list rejects a kind it omits", `{"type":["string","null"]}`, `3`, false},
		{"an unknown type matches nothing", `{"type":"decimal"}`, `3`, false},
		{"integer rejects a fraction", `{"type":"integer"}`, `1.5`, false},
		{"minimum rejects a smaller integer", `{"type":"integer","minimum":1}`, `0`, false},
		{"minimum accepts the bound", `{"type":"integer","minimum":1}`, `1`, true},
		{"enum accepts a listed value", `{"type":"string","enum":["a","b"]}`, `"a"`, true},
		{"enum rejects an unlisted value", `{"type":"string","enum":["a","b"]}`, `"c"`, false},
		{"anyOf accepts a matching branch", `{"anyOf":[{"type":"string"},{"type":"null"}]}`, `null`, true},
		{"anyOf rejects when no branch matches", `{"anyOf":[{"type":"string"},{"type":"null"}]}`, `3`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := wireschema.Validate([]byte(tc.schema), []byte(tc.doc))
			if tc.valid && err != nil {
				t.Errorf("rejected %s against %s: %v", tc.doc, tc.schema, err)
			}
			if !tc.valid && err == nil {
				t.Errorf("accepted %s against %s", tc.doc, tc.schema)
			}
		})
	}
}
