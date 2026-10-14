package wire

import (
	"bytes"
	"os"
	"reflect"
	"testing"
)

func TestCheckedInSchemasMatchGenerated(t *testing.T) {
	t.Parallel()
	cases := []struct {
		path string
		got  []byte
	}{
		{"../../../schema/run.v3.json", RunJSONSchema()},
		{"../../../schema/event.v2.json", EventJSONSchema()},
	}
	for _, c := range cases {
		want, err := os.ReadFile(c.path)
		if err != nil {
			t.Errorf("read %s: %v (run go run ./tools/genschema from the module root)", c.path, err)
			continue
		}
		if !bytes.Equal(want, c.got) {
			t.Errorf("%s differs from the schema generated from Go wire types; run go run ./tools/genschema", c.path)
		}
	}
}

// TestNilPointerWithoutOmitemptyIsDeclaredNullable pins the generator rule behind the published schema: a
// pointer the encoder writes as null when nil is declared to accept null, and a pointer it omits is not.
func TestNilPointerWithoutOmitemptyIsDeclaredNullable(t *testing.T) {
	t.Parallel()
	type holder struct {
		Text     *string      `json:"text"`
		Progress *ProgressDoc `json:"progress"`
		Omitted  *string      `json:"omitted,omitempty"`
	}
	g := &schemaGen{defs: map[string]any{}}
	props := g.objectOf(reflect.TypeFor[holder](), nil)["properties"].(map[string]any)

	if got := props["text"].(map[string]any)["type"]; !reflect.DeepEqual(got, []string{"string", "null"}) {
		t.Errorf("nullable string pointer has type %v, want [string null]", got)
	}
	if _, ok := props["progress"].(map[string]any)["anyOf"]; !ok {
		t.Errorf("nullable struct pointer is %v, want an anyOf with null", props["progress"])
	}
	if got := props["omitted"].(map[string]any)["type"]; got != "string" {
		t.Errorf("omitempty pointer has type %v, want plain string", got)
	}
}
