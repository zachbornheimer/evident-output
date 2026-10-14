package wire

//go:generate go run ../../../tools/genschema

import (
	"encoding/json"
	"reflect"
	"strings"
	"time"
)

const (
	runSchemaID     = "https://evident-output.dev/schema/v3/run.json"
	eventSchemaID   = "https://evident-output.dev/schema/v2/event.json"
	jsonSchemaDraft = "https://json-schema.org/draft/2020-12/schema"
)

// RunJSONSchema is the JSON Schema for RunDocument, the evo.run envelope.
func RunJSONSchema() []byte {
	return generateSchema(runSchemaID, "Evident Output final JSON v3 (evo.run)", reflect.TypeFor[RunDocument](), map[string]any{
		"object":         map[string]any{"const": RunObject},
		"schema_version": map[string]any{"const": RunSchemaVersion},
	})
}

// EventJSONSchema is the JSON Schema for EventDocument, one evo.event line.
func EventJSONSchema() []byte {
	return generateSchema(eventSchemaID, "Evident Output JSONL event v2 (evo.event)", reflect.TypeFor[EventDocument](), map[string]any{
		"object":         map[string]any{"const": EventObject},
		"schema_version": map[string]any{"const": EventSchemaVersion},
		"seq":            map[string]any{"type": "integer", "minimum": 1},
	}, documentedPayloads)
}

// documentedPayloads are the event types whose payload the schema binds, and the wire type each payload is.
var documentedPayloads = []payloadSchema{
	{EventDefinitionFinished, reflect.TypeFor[DefinitionFinishedPayload]()},
	{EventVerificationObserved, reflect.TypeFor[VerificationDoc]()},
}

// payloadSchema binds the payload of one event type to the wire type that describes it.
type payloadSchema struct {
	event   string
	payload reflect.Type
}

// conditions is the schema's allOf: one "when the type is this event, the payload is this shape" clause per
// documented payload. The payload shape is a $defs entry, so a strict reading rejects an undeclared field.
func (g *schemaGen) conditions(payloads []payloadSchema) []any {
	clauses := make([]any, 0, len(payloads))
	for _, p := range payloads {
		clauses = append(clauses, map[string]any{
			"if": map[string]any{
				"type":       "object",
				"properties": map[string]any{"type": map[string]any{"const": p.event}},
				"required":   []string{"type"},
			},
			"then": map[string]any{
				"type":       "object",
				"properties": map[string]any{"payload": g.property(p.payload)},
				"required":   []string{"payload"},
			},
		})
	}
	return clauses
}

func generateSchema(id, title string, root reflect.Type, overrides map[string]any, payloads ...[]payloadSchema) []byte {
	g := &schemaGen{defs: map[string]any{}}
	schema := g.objectOf(root, overrides)
	for _, documented := range payloads {
		schema["allOf"] = g.conditions(documented)
	}
	schema["$schema"] = jsonSchemaDraft
	schema["$id"] = id
	schema["title"] = title
	if len(g.defs) > 0 {
		schema["$defs"] = g.defs
	}
	body, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		panic("wire: marshal JSON schema: " + err.Error())
	}
	return append(body, '\n')
}

type schemaGen struct {
	defs map[string]any
}

// objectOf describes struct t. overrides replace named properties of t itself only: a nested struct's
// field that shares a name (an effect's "object") is its own property, never the envelope's.
func (g *schemaGen) objectOf(t reflect.Type, overrides map[string]any) map[string]any {
	t = unwrap(t)
	props := map[string]any{}
	required := make([]string, 0, t.NumField())
	for i := range t.NumField() {
		field := t.Field(i)
		name, omitempty, skip := jsonField(field)
		if skip {
			continue
		}
		if override, ok := overrides[name]; ok {
			props[name] = override
		} else {
			props[name] = g.property(field.Type)
		}
		if !omitempty {
			required = append(required, name)
		}
	}
	schema := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func (g *schemaGen) property(t reflect.Type) any {
	t = unwrap(t)
	if t.Kind() == reflect.Struct && t != reflect.TypeFor[time.Time]() {
		ref := g.define(t)
		return map[string]any{"$ref": "#/$defs/" + ref}
	}
	return g.typeSchema(t)
}

func (g *schemaGen) define(t reflect.Type) string {
	name := defName(t)
	if _, exists := g.defs[name]; exists {
		return name
	}
	g.defs[name] = map[string]any{"type": "object"}
	g.defs[name] = g.objectOf(t, nil)
	return name
}

func (g *schemaGen) typeSchema(t reflect.Type) any {
	t = unwrap(t)
	switch t.Kind() {
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}
	case reflect.Slice, reflect.Array:
		return map[string]any{"type": "array", "items": g.elemSchema(t.Elem())}
	case reflect.Map:
		return map[string]any{"type": "object"}
	case reflect.Struct:
		if t == reflect.TypeFor[time.Time]() {
			return map[string]any{"type": "string"}
		}
		return g.objectOf(t, nil)
	default:
		return map[string]any{}
	}
}

func (g *schemaGen) elemSchema(t reflect.Type) any {
	t = unwrap(t)
	if t.Kind() == reflect.Struct && t != reflect.TypeFor[time.Time]() {
		return map[string]any{"$ref": "#/$defs/" + g.define(t)}
	}
	return g.typeSchema(t)
}

func unwrap(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

func jsonField(field reflect.StructField) (name string, omitempty, skip bool) {
	if !field.IsExported() {
		return "", false, true
	}
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", false, true
	}
	if tag == "" {
		return field.Name, false, false
	}
	name, rest, _ := strings.Cut(tag, ",")
	if name == "" || name == "-" {
		return "", false, true
	}
	return name, strings.Contains(rest, "omitempty"), false
}

func defName(t reflect.Type) string {
	name := strings.TrimSuffix(t.Name(), "Doc")
	name = strings.TrimSuffix(name, "Document")
	if name == "RunData" {
		return "data"
	}
	if name == "EvidencePhase" {
		return "evidencePhase"
	}
	if name == "TrackedResource" {
		return "trackedResource"
	}
	if name == "" {
		return t.Name()
	}
	return strings.ToLower(name[:1]) + name[1:]
}
