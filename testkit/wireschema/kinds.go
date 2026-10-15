package wireschema

import (
	"encoding/json"
	"slices"
	"strings"
)

// checkType holds value to the schema's "type": one name, or a list of which value must match one. A value
// of the wrong kind fails, including JSON null against a type list without "null".
func (v *validator) checkType(path string, schema map[string]any, value any) {
	want := schemaTypes(schema["type"])
	if len(want) == 0 {
		return
	}
	idx := slices.IndexFunc(want, func(name string) bool { return kindMatches(name, value) })
	if idx < 0 {
		v.fail(path, "want %s, got %v (%T)", strings.Join(want, "|"), value, value)
		return
	}
	switch want[idx] {
	case "object":
		v.checkObject(path, schema, value)
	case "array":
		v.checkArray(path, schema, value)
	case "integer", "number":
		v.checkMinimum(path, schema, value)
	}
}

// checkMinimum applies the schema's "minimum" to a number.
func (v *validator) checkMinimum(path string, schema map[string]any, value any) {
	minimum, ok := schema["minimum"].(float64)
	if got, isNumber := value.(float64); ok && isNumber && got < minimum {
		v.fail(path, "%v is below the minimum %v", got, minimum)
	}
}

// checkEnum holds value to the schema's closed list of allowed values.
func (v *validator) checkEnum(path string, schema map[string]any, value any) {
	allowed, ok := schema["enum"].([]any)
	if !ok {
		return
	}
	if !slices.ContainsFunc(allowed, func(candidate any) bool { return sameJSON(candidate, value) }) {
		v.fail(path, "value %v is not one of the enum %v", value, allowed)
	}
}

// checkAnyOf requires value to satisfy at least one branch of the schema's anyOf.
func (v *validator) checkAnyOf(path string, schema map[string]any, value any) {
	branches, ok := schema["anyOf"].([]any)
	if !ok {
		return
	}
	for _, raw := range branches {
		branch, isSchema := raw.(map[string]any)
		if !isSchema {
			continue
		}
		probe := &validator{defs: v.defs}
		probe.check(path, branch, value)
		if len(probe.errs) == 0 {
			return
		}
	}
	v.fail(path, "value %v matches none of the anyOf branches", value)
}

// schemaTypes is a schema's "type" as a list: the draft allows one name or an array of names.
func schemaTypes(raw any) []string {
	if name, ok := raw.(string); ok {
		return []string{name}
	}
	return stringSlice(raw)
}

// kindMatches reports whether a decoded JSON value is of the JSON Schema type name. An unknown name matches nothing.
func kindMatches(name string, value any) bool {
	switch name {
	case "null":
		return value == nil
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "number":
		_, ok := value.(float64)
		return ok
	case "integer":
		return isJSONInteger(value)
	}
	return false
}

// isJSONInteger reports whether a decoded encoding/json float64 holds a
// whole number — JSON has no separate integer type, so this is the
// faithful way to check "integer" against a value that was always
// unmarshaled as float64.
func isJSONInteger(value any) bool {
	f, ok := value.(float64)
	return ok && f == float64(int64(f))
}

// sameJSON reports whether two decoded JSON values are equal.
func sameJSON(a, b any) bool {
	left, errLeft := json.Marshal(a)
	right, errRight := json.Marshal(b)
	return errLeft == nil && errRight == nil && string(left) == string(right)
}
