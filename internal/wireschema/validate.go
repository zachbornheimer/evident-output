// Package wireschema validates a rendered JSON document against evo's own
// published JSON Schema (schema/output.v1.json) — the facade that makes the
// schema file an enforced gate instead of prose nobody checks (P8: "today
// nothing references that file — make it a gate").
//
// It implements only the subset of JSON Schema (draft 2020-12) the wire
// schema actually uses: object/array/string/integer/boolean, required,
// properties, items, const, and $ref into $defs. That is a deliberate
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
	if want, ok := schema["const"]; ok {
		if fmt.Sprint(value) != fmt.Sprint(want) {
			v.fail(path, "const mismatch: got %v, want %v", value, want)
		}
		return
	}
	wantType, _ := schema["type"].(string)
	switch wantType {
	case "object":
		v.checkObject(path, schema, value)
	case "array":
		v.checkArray(path, schema, value)
	case "string":
		if _, ok := value.(string); !ok {
			v.fail(path, "want string, got %T", value)
		}
	case "integer":
		if !isJSONInteger(value) {
			v.fail(path, "want integer, got %v (%T)", value, value)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			v.fail(path, "want boolean, got %T", value)
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

// isJSONInteger reports whether a decoded encoding/json float64 holds a
// whole number — JSON has no separate integer type, so this is the
// faithful way to check "integer" against a value that was always
// unmarshaled as float64.
func isJSONInteger(value any) bool {
	f, ok := value.(float64)
	return ok && f == float64(int64(f))
}
