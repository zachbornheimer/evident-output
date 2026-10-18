// Package wireschema validates a rendered JSON document against evo's own
// published JSON Schema (schema/run.v3.json) — the facade that makes the
// schema file an enforced gate instead of prose nobody checks (P8: "today
// nothing references that file — make it a gate").
//
// It implements only the subset of JSON Schema (draft 2020-12) the wire
// schema actually uses, with standard semantics for it: type (one name or a
// list; JSON null matches only "null"), required, properties, items, const,
// enum, minimum, anyOf, $ref into $defs, and allOf of if/then clauses (how the
// evo.event schema binds each documented payload to its event type). That is a deliberate
// boundary, not a shortcut — evo owns its own schema and controls what it
// writes into it, so a full general-purpose validator is a dependency this
// package doesn't need.
package wireschema

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Strict returns schema with "additionalProperties": false injected into
// every $def that declares "properties", so Validate(Strict(schema), doc)
// rejects any field the schema does not declare.
//
// The published schema file (schema/run.v3.json)
// stays permissive on disk — additionalProperties:false there would turn
// every future additive field into a breaking schema_version bump, which
// contradicts the "schema compatibility tests cover additions" contract.
// Strict exists so tests can still catch the regression that motivated
// wanting it in the first place: a schema that silently drops a field it
// used to declare (a reverted addition) would otherwise validate any
// document, declared field or not. Callers pass Strict's output to
// Validate instead of relying on the on-disk schema carrying the flag.
func Strict(schema []byte) ([]byte, error) {
	var root any
	if err := json.Unmarshal(schema, &root); err != nil {
		return nil, fmt.Errorf("wireschema: parse schema: %w", err)
	}
	injectAdditionalPropertiesFalse(root)
	out, err := json.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("wireschema: marshal strict schema: %w", err)
	}
	return out, nil
}

// injectAdditionalPropertiesFalse walks every object node reachable in a
// decoded JSON Schema document and sets additionalProperties:false on each
// one that declares "properties" — the shape checkObject treats as an
// opt-in strict $def.
func injectAdditionalPropertiesFalse(node any) {
	switch v := node.(type) {
	case map[string]any:
		if _, hasProps := v["properties"]; hasProps {
			v["additionalProperties"] = false
		}
		for key, child := range v {
			if key == "if" || key == "then" {
				continue // a condition and its consequence describe part of an object, not all of it
			}
			injectAdditionalPropertiesFalse(child)
		}
	case []any:
		for _, child := range v {
			injectAdditionalPropertiesFalse(child)
		}
	}
}

// Validate reports every way doc fails to conform to schema, or nil when it
// conforms. Both arguments are raw JSON: schema is a JSON Schema document,
// doc is the value being checked against it.
func Validate(schema, doc []byte) error {
	var schemaVal, docVal any
	if err := json.Unmarshal(schema, &schemaVal); err != nil {
		return fmt.Errorf("wireschema: parse schema: %w", err)
	}
	if err := json.Unmarshal(doc, &docVal); err != nil {
		return fmt.Errorf("wireschema: parse document: %w", err)
	}
	root, ok := schemaVal.(map[string]any)
	if !ok {
		return fmt.Errorf("wireschema: schema root is not an object")
	}
	defs, _ := root["$defs"].(map[string]any)
	v := &validator{defs: defs}
	v.check("$", root, docVal)
	if len(v.errs) == 0 {
		return nil
	}
	sort.Strings(v.errs)
	return fmt.Errorf("wireschema: %d violation(s):\n%s", len(v.errs), strings.Join(v.errs, "\n"))
}

type validator struct {
	defs map[string]any
	errs []string
}

func (v *validator) fail(path, format string, args ...any) {
	v.errs = append(v.errs, path+": "+fmt.Sprintf(format, args...))
}

// check validates value against schema at path, resolving $ref first.
func (v *validator) check(path string, schema map[string]any, value any) {
	if ref, ok := schema["$ref"].(string); ok {
		resolved, ok := v.resolveRef(ref)
		if !ok {
			v.fail(path, "unresolved $ref %q", ref)
			return
		}
		schema = resolved
	}
	v.checkConditions(path, schema, value)
	if want, ok := schema["const"]; ok {
		if !sameJSON(value, want) {
			v.fail(path, "const mismatch: got %v, want %v", value, want)
		}
	}
	v.checkAnyOf(path, schema, value)
	v.checkEnum(path, schema, value)
	v.checkType(path, schema, value)
}

// checkConditions applies schema's allOf clauses: each clause's "then" binds value only when its "if" holds.
func (v *validator) checkConditions(path string, schema map[string]any, value any) {
	clauses, _ := schema["allOf"].([]any)
	for _, raw := range clauses {
		clause, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		condition, _ := clause["if"].(map[string]any)
		consequence, _ := clause["then"].(map[string]any)
		if condition == nil || consequence == nil {
			continue
		}
		probe := &validator{defs: v.defs}
		probe.check(path, condition, value)
		if len(probe.errs) == 0 {
			v.check(path, consequence, value)
		}
	}
}

func (v *validator) resolveRef(ref string) (map[string]any, bool) {
	const prefix = "#/$defs/"
	if !strings.HasPrefix(ref, prefix) {
		return nil, false
	}
	name := strings.TrimPrefix(ref, prefix)
	def, ok := v.defs[name].(map[string]any)
	return def, ok
}

func (v *validator) checkObject(path string, schema map[string]any, value any) {
	obj, ok := value.(map[string]any)
	if !ok {
		v.fail(path, "want object, got %T", value)
		return
	}
	for _, req := range stringSlice(schema["required"]) {
		if _, present := obj[req]; !present {
			v.fail(path, "missing required field %q", req)
		}
	}
	props, _ := schema["properties"].(map[string]any)
	for name, propSchema := range props {
		fieldVal, present := obj[name]
		if !present {
			continue // optional field omitted entirely — valid
		}
		ps, ok := propSchema.(map[string]any)
		if !ok {
			continue
		}
		v.check(path+"."+name, ps, fieldVal)
	}
	// additionalProperties:false makes a schema strict: a document field
	// this schema does not declare in "properties" is a violation. This is
	// opt-in (schema["additionalProperties"] absent, or explicitly true,
	// keeps today's permissive behavior) rather than a default, so
	// existing schemas that haven't been reviewed for completeness don't
	// start failing — but any $def that does set it to false now actually
	// gates unknown fields, closing the gap where a reverted schema
	// addition still validated against a document that kept emitting the
	// field.
	allowsAdditional, isBool := schema["additionalProperties"].(bool)
	if !isBool || allowsAdditional {
		return
	}
	names := make([]string, 0, len(obj))
	for name := range obj {
		if _, declared := props[name]; declared {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		v.fail(path, "additional property %q not declared in schema", name)
	}
}

func (v *validator) checkArray(path string, schema map[string]any, value any) {
	arr, ok := value.([]any)
	if !ok {
		v.fail(path, "want array, got %T", value)
		return
	}
	itemSchema, _ := schema["items"].(map[string]any)
	if itemSchema == nil {
		return // array declared with no item shape — any element is valid
	}
	for i, item := range arr {
		v.check(fmt.Sprintf("%s[%d]", path, i), itemSchema, item)
	}
}

func stringSlice(v any) []string {
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, e := range raw {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
