package wireschema_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/testkit/wireschema"
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

// TestValidate_AdditionalPropertiesFalseAcceptsDeclaredFields is half of
// the ZYS-823 review-gap fix: checkObject previously only checked declared
// properties and never looked at "additionalProperties", so a schema that
// dropped a field it used to declare (a reverted addition) still validated
// against any document — the exact false-negative
// internal/wire/compat_test.go's TestSchemaCompat_*ValidatesAgainstSchema
// tests were meant to catch. This half proves declared-only documents
// still pass once additionalProperties:false is enforced.
func TestValidate_AdditionalPropertiesFalseAcceptsDeclaredFields(t *testing.T) {
	schema := `{"type":"object","additionalProperties":false,"properties":{"name":{"type":"string"}}}`
	if err := wireschema.Validate([]byte(schema), []byte(`{"name":"x"}`)); err != nil {
		t.Fatalf("unexpected violation for declared-only fields: %v", err)
	}
}

// TestValidate_AdditionalPropertiesFalseRejectsUnknownField is the other
// half: a field the schema does not declare must fail once
// additionalProperties:false opts a schema into strict checking.
func TestValidate_AdditionalPropertiesFalseRejectsUnknownField(t *testing.T) {
	schema := `{"type":"object","additionalProperties":false,"properties":{"name":{"type":"string"}}}`
	err := wireschema.Validate([]byte(schema), []byte(`{"name":"x","surprise":"field"}`))
	if err == nil {
		t.Fatal("expected violation for a field additionalProperties:false does not declare")
	}
	if !strings.Contains(err.Error(), `"surprise"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestValidate_NoAdditionalPropertiesDirectiveAllowsUnknownField documents
// that omitting "additionalProperties" keeps today's permissive behavior —
// schemas that have not opted in stay unaffected by this fix.
func TestValidate_NoAdditionalPropertiesDirectiveAllowsUnknownField(t *testing.T) {
	err := wireschema.Validate([]byte(minimalSchema), []byte(`{"name":"x","count":3,"extra":"ok"}`))
	if err != nil {
		t.Fatalf("unexpected violation: %v", err)
	}
}

// TestStrict_AcceptsAdditiveFieldAgainstPlainSchema proves the ZYS-823
// contract Strict exists to protect: a plain (non-strict) schema, the
// shape schema/run.v3.json is published as,
// keeps validating a document that carries a field the schema never
// declared — an additive field is not a breaking change against the
// published schema.
func TestStrict_AcceptsAdditiveFieldAgainstPlainSchema(t *testing.T) {
	err := wireschema.Validate([]byte(minimalSchema), []byte(`{"name":"x","count":3,"new_field":"added later"}`))
	if err != nil {
		t.Fatalf("plain schema must accept an additive field, got: %v", err)
	}
}

// TestStrict_RejectsUndeclaredFieldEvenWhenSourceSchemaOmitsIt proves
// Strict itself performs the injection: a schema on disk that never sets
// additionalProperties still rejects an unknown field once passed through
// Strict, which is what lets compat tests catch a reverted field addition
// without the published schema paying the strictness cost.
func TestStrict_RejectsUndeclaredFieldEvenWhenSourceSchemaOmitsIt(t *testing.T) {
	strict, err := wireschema.Strict([]byte(minimalSchema))
	if err != nil {
		t.Fatalf("wireschema.Strict: %v", err)
	}
	err = wireschema.Validate(strict, []byte(`{"name":"x","count":3,"surprise":"field"}`))
	if err == nil {
		t.Fatal("expected Strict schema to reject a field minimalSchema never declared")
	}
	if !strings.Contains(err.Error(), `"surprise"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestStrict_StillAcceptsDeclaredFields proves Strict does not change
// what a conforming document looks like — only what an undeclared field
// does.
func TestStrict_StillAcceptsDeclaredFields(t *testing.T) {
	strict, err := wireschema.Strict([]byte(minimalSchema))
	if err != nil {
		t.Fatalf("wireschema.Strict: %v", err)
	}
	if err := wireschema.Validate(strict, []byte(`{"name":"x","count":3,"tags":["a"]}`)); err != nil {
		t.Fatalf("unexpected violation for declared fields: %v", err)
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

// conditionalSchema binds payload to a shape only for the event type that names it.
const conditionalSchema = `{
  "type": "object",
  "properties": {"type": {"type": "string"}, "payload": {"type": "object"}},
  "allOf": [{
    "if": {"type": "object", "properties": {"type": {"const": "done"}}, "required": ["type"]},
    "then": {"type": "object", "properties": {"payload": {"$ref": "#/$defs/done"}}, "required": ["payload"]}
  }],
  "$defs": {"done": {"type": "object", "required": ["failed"], "properties": {"failed": {"type": "boolean"}}}}
}`

func TestValidate_AppliesAConditionalClauseOnlyWhenItsConditionHolds(t *testing.T) {
	schema := []byte(conditionalSchema)
	cases := []struct {
		name, doc string
		valid     bool
	}{
		{"matching type with a conforming payload", `{"type":"done","payload":{"failed":false}}`, true},
		{"matching type with a mistyped payload", `{"type":"done","payload":{"failed":"no"}}`, false},
		{"matching type without a payload", `{"type":"done"}`, false},
		{"other type is not bound by the clause", `{"type":"other","payload":{"anything":1}}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := wireschema.Validate(schema, []byte(tc.doc))
			if tc.valid && err != nil {
				t.Fatalf("unexpected violation: %v", err)
			}
			if !tc.valid && err == nil {
				t.Fatal("expected a violation")
			}
		})
	}
}

func TestStrict_LeavesAConditionOpenSoItMatchesTheWholeObject(t *testing.T) {
	strict, err := wireschema.Strict([]byte(conditionalSchema))
	if err != nil {
		t.Fatalf("Strict: %v", err)
	}
	if err := wireschema.Validate(strict, []byte(`{"type":"done","payload":{"failed":true}}`)); err != nil {
		t.Fatalf("a conforming document fails under Strict: %v", err)
	}
	if err := wireschema.Validate(strict, []byte(`{"type":"done","payload":{"failed":true,"extra":1}}`)); err == nil {
		t.Fatal("Strict accepted an undeclared payload field")
	}
}
