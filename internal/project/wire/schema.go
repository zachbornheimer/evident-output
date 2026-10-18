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
		"type":           map[string]any{"type": "string", "enum": EventTypes()},
	}, documentedPayloads)
}

// EventPayloader is a payload type: it builds its own evo.event payload map.
type EventPayloader interface {
	EventPayload() map[string]any
}

// documentedPayloads binds every event type to the payload type that describes it, in the order the
// constants are declared. The "type" enum is these names, so an event type is published exactly when its
// payload is bound; TestEventSchemaBindsEveryEventType holds the list to the Event* constants.
var documentedPayloads = []payloadSchema{
	bind[RunStartedPayload](EventRunStarted),
	bind[CollectionDeclaredPayload](EventCollectionDeclared),
	bind[TaskDeclaredPayload](EventTaskDeclared),
	bind[EmptyPayload](EventTaskEligible),
	bind[EvidenceEvaluatedPayload](EventEvidenceEvaluated),
	bind[EmptyPayload](EventTaskStarted),
	bind[EmptyPayload](EventDefinitionStarted),
	bind[OperationObservedPayload](EventOperationStarted),
	bind[OperationObservedPayload](EventOperationSkippedCurrent),
	bind[TrackedResourceObservedPayload](EventTrackedResourceObserved),
	bind[BasisFingerprintedPayload](EventBasisFingerprinted),
	bind[VerificationDoc](EventVerificationObserved),
	bind[FactDoc](EventFactRecorded),
	bind[ProblemRecordedPayload](EventWarningRecorded),
	bind[ProblemRecordedPayload](EventProblemRecorded),
	bind[EffectPayload](EventEffectPlanned),
	bind[EffectPayload](EventEffectCommitted),
	bind[ManifestTaskCommittedPayload](EventManifestTaskCommitted),
	bind[OperationFinishedPayload](EventOperationFinished),
	bind[DefinitionFinishedPayload](EventDefinitionFinished),
	bind[TaskFinishedPayload](EventTaskFinished),
	bind[RunFinishedPayload](EventRunFinished),
	bind[ResultWrittenPayload](EventResultWritten),
}

// EventTypes are the event types an evo.event line may carry: the closed enum of the schema's "type".
func EventTypes() []string {
	types := make([]string, len(documentedPayloads))
	for i, bound := range documentedPayloads {
		types[i] = bound.event
	}
	return types
}

// payloadSchema binds the payload of one event type to the wire type that describes it.
type payloadSchema struct {
	event   string
	payload reflect.Type
}

func bind[P EventPayloader](event string) payloadSchema {
	return payloadSchema{event, reflect.TypeFor[P]()}
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
			"then": g.payloadClause(p.payload),
		})
	}
	return clauses
}

// payloadClause is what an event's payload must be: the bound shape, present unless every field of it is
// optional (the writer omits an empty payload).
func (g *schemaGen) payloadClause(payload reflect.Type) map[string]any {
	clause := map[string]any{
		"type":       "object",
		"properties": map[string]any{"payload": g.property(payload)},
	}
	for _, field := range jsonFields(payload) {
		if !field.omitempty {
			clause["required"] = []string{"payload"}
			break
		}
	}
	return clause
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
	var required []string
	for _, field := range jsonFields(t) {
		if override, ok := overrides[field.name]; ok {
			props[field.name] = override
		} else {
			props[field.name] = g.property(field.typ)
		}
		if !field.omitempty {
			required = append(required, field.name)
			if field.typ.Kind() == reflect.Pointer {
				props[field.name] = orNull(props[field.name])
			}
		}
	}
	schema := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// orNull widens schema to also accept JSON null: the encoder writes a nil pointer that has no omitempty as
// null, so the schema must say so for a stock validator to accept the document.
func orNull(schema any) any {
	described, ok := schema.(map[string]any)
	if !ok {
		return schema
	}
	if name, isType := described["type"].(string); isType {
		described["type"] = []string{name, "null"}
		return described
	}
	return map[string]any{"anyOf": []any{described, map[string]any{"type": "null"}}}
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
		if set, closed := reflect.Zero(t).Interface().(closedSet); closed {
			return map[string]any{"type": "string", "enum": set.Values()}
		}
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

// closedSet is a wire string type that takes one of a fixed set of values; the schema publishes the set as
// an enum, so a stock validator rejects any other.
type closedSet interface{ Values() []string }

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

// wireField is one JSON property of a struct: its name, whether the encoder omits it when empty, and its type.
type wireField struct {
	name      string
	omitempty bool
	typ       reflect.Type
}

// jsonFields are the JSON properties of struct t in declaration order. An embedded struct with no json tag
// contributes its fields, as encoding/json flattens it.
func jsonFields(t reflect.Type) []wireField {
	fields := make([]wireField, 0, t.NumField())
	for i := range t.NumField() {
		field := t.Field(i)
		if embedded := unwrap(field.Type); field.Anonymous && field.Tag.Get("json") == "" && embedded.Kind() == reflect.Struct {
			fields = append(fields, jsonFields(embedded)...)
			continue
		}
		name, omitempty, skip := jsonField(field)
		if !skip {
			fields = append(fields, wireField{name, omitempty, field.Type})
		}
	}
	return fields
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
