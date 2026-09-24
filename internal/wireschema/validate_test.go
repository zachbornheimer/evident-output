package wireschema_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/wireschema"
)

const minimalSchema = `{
  "type": "object",
  "required": ["name", "count"],
  "properties": {
    "name": {"type": "string"},
    "count": {"type": "integer"},
    "tags": {"type": "array", "items": {"type": "string"}}
  }
}`

func TestValidate_AcceptsConformingDocument(t *testing.T) {
	err := wireschema.Validate([]byte(minimalSchema), []byte(`{"name":"x","count":3,"tags":["a","b"]}`))
	if err != nil {
		t.Fatalf("unexpected violation: %v", err)
	}
}

func TestValidate_RejectsMissingRequiredField(t *testing.T) {
	err := wireschema.Validate([]byte(minimalSchema), []byte(`{"name":"x"}`))
	if err == nil {
		t.Fatal("expected violation for missing required field")
	}
	if !strings.Contains(err.Error(), `missing required field "count"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_RejectsWrongType(t *testing.T) {
	err := wireschema.Validate([]byte(minimalSchema), []byte(`{"name":"x","count":"three"}`))
	if err == nil {
		t.Fatal("expected violation for wrong type")
	}
	if !strings.Contains(err.Error(), "want integer") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_RejectsArrayItemTypeMismatch(t *testing.T) {
	err := wireschema.Validate([]byte(minimalSchema), []byte(`{"name":"x","count":1,"tags":["a",2]}`))
	if err == nil {
		t.Fatal("expected violation for array item type mismatch")
	}
}

func TestValidate_ConstMismatchFails(t *testing.T) {
	schema := `{"type":"object","properties":{"v":{"const":"0.4"}}}`
	if err := wireschema.Validate([]byte(schema), []byte(`{"v":"0.3"}`)); err == nil {
		t.Fatal("expected const mismatch to fail")
	}
	if err := wireschema.Validate([]byte(schema), []byte(`{"v":"0.4"}`)); err != nil {
		t.Fatalf("unexpected violation: %v", err)
	}
}

const defsSchema = `{
  "type": "object",
  "properties": {"payload": {"type": "object"}},
  "$defs": {
    "rate": {
      "type": "object",
      "required": ["share"],
      "properties": {"share": {"type": "number"}}
    }
  }
}`

func TestValidate_NumberAcceptsFractionsAndRejectsStrings(t *testing.T) {
	if err := wireschema.ValidateDef([]byte(defsSchema), []byte(`{"share":0.25}`), "rate"); err != nil {
		t.Fatalf("unexpected violation: %v", err)
	}
	if err := wireschema.ValidateDef([]byte(defsSchema), []byte(`{"share":"0.25"}`), "rate"); err == nil || !strings.Contains(err.Error(), "want number") {
		t.Fatalf("want a number violation, got %v", err)
	}
}

func TestValidateDef_ChecksTheNamedDefNotTheRoot(t *testing.T) {
	if err := wireschema.ValidateDef([]byte(defsSchema), []byte(`{}`), "rate"); err == nil || !strings.Contains(err.Error(), `missing required field "share"`) {
		t.Fatalf("want the def's required field enforced, got %v", err)
	}
	if err := wireschema.ValidateDef([]byte(defsSchema), []byte(`{}`), "absent"); err == nil || !strings.Contains(err.Error(), "unresolved $ref") {
		t.Fatalf("want an unresolved def reported, got %v", err)
	}
}
