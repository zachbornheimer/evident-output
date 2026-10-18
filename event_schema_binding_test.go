package evo_test

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
	"github.com/zachbornheimer/evident-output/v2/internal/project/wire"
	"github.com/zachbornheimer/evident-output/v2/testkit/wireschema"
)

const (
	wireEventFile      = "internal/project/wire/event.go"
	firstEventConstant = "EventRunStarted"
)

// declaredEventTypes are the values of the wire package's event family constants, read from the source so a
// constant added there is held to the schema without anyone listing it twice.
func declaredEventTypes(t *testing.T) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), wireEventFile, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", wireEventFile, err)
	}
	for _, decl := range file.Decls {
		group, ok := decl.(*ast.GenDecl)
		if !ok || group.Tok != token.CONST || !declares(group, firstEventConstant) {
			continue
		}
		var types []string
		for _, spec := range group.Specs {
			for _, value := range spec.(*ast.ValueSpec).Values {
				literal, ok := value.(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					t.Fatalf("%s: event constant is not a string literal", wireEventFile)
				}
				text, err := strconv.Unquote(literal.Value)
				if err != nil {
					t.Fatalf("%s: unquote %s: %v", wireEventFile, literal.Value, err)
				}
				types = append(types, text)
			}
		}
		return types
	}
	t.Fatalf("%s has no const block declaring %s", wireEventFile, firstEventConstant)
	return nil
}

func declares(group *ast.GenDecl, name string) bool {
	return slices.ContainsFunc(group.Specs, func(spec ast.Spec) bool {
		return slices.ContainsFunc(spec.(*ast.ValueSpec).Names, func(id *ast.Ident) bool { return id.Name == name })
	})
}

// eventSchemaBindings reads the published schema's "type" enum and the payload each event type is bound to.
func eventSchemaBindings(t *testing.T) (enum []string, bound map[string]bool) {
	t.Helper()
	body, err := os.ReadFile(publishedEventSchema)
	if err != nil {
		t.Fatalf("read %s: %v", publishedEventSchema, err)
	}
	var schema struct {
		Properties struct {
			Type struct {
				Enum []string `json:"enum"`
			} `json:"type"`
		} `json:"properties"`
		AllOf []struct {
			If struct {
				Properties struct {
					Type struct {
						Const string `json:"const"`
					} `json:"type"`
				} `json:"properties"`
			} `json:"if"`
			Then struct {
				Properties struct {
					Payload map[string]any `json:"payload"`
				} `json:"properties"`
			} `json:"then"`
		} `json:"allOf"`
	}
	if err := json.Unmarshal(body, &schema); err != nil {
		t.Fatalf("decode %s: %v", publishedEventSchema, err)
	}
	bound = map[string]bool{}
	for _, clause := range schema.AllOf {
		name := clause.If.Properties.Type.Const
		if bound[name] {
			t.Errorf("%s binds %q twice", publishedEventSchema, name)
		}
		bound[name] = len(clause.Then.Properties.Payload) > 0
	}
	return schema.Properties.Type.Enum, bound
}

// TestEventSchemaBindsEveryEventType holds schema/event.v2.json to the wire package's event constants: each
// has a payload binding, "type" is a closed enum of exactly those names, and every event real programs
// write (all of them are written) validates strictly against it.
func TestEventSchemaBindsEveryEventType(t *testing.T) {
	declared := declaredEventTypes(t)
	enum, bound := eventSchemaBindings(t)

	for _, name := range declared {
		if !bound[name] {
			t.Errorf("event type %q has no payload binding in %s", name, publishedEventSchema)
		}
	}
	for name := range bound {
		if !slices.Contains(declared, name) {
			t.Errorf("%s binds %q, which no Event constant declares", publishedEventSchema, name)
		}
	}
	if !slices.Equal(slices.Sorted(slices.Values(enum)), slices.Sorted(slices.Values(declared))) {
		t.Errorf("%s \"type\" enum is\n%v\nwant exactly the Event constants\n%v", publishedEventSchema, enum, declared)
	}
	if !slices.Equal(slices.Sorted(slices.Values(wire.EventTypes())), slices.Sorted(slices.Values(declared))) {
		t.Errorf("wire.EventTypes() is\n%v\nwant exactly the Event constants\n%v", wire.EventTypes(), declared)
	}

	emitted := map[string]bool{}
	for _, out := range realMachineOutputs(t) {
		if out.mode != evo.JSONL {
			continue
		}
		for line := range strings.SplitSeq(strings.TrimSpace(out.stdout), "\n") {
			validateAgainstPublished(t, publishedEventSchema, []byte(line))
			var event struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				t.Fatalf("event is not JSON: %v\n%s", err, line)
			}
			emitted[event.Type] = true
		}
	}
	for _, name := range declared {
		if !emitted[name] && os.Geteuid() != 0 {
			t.Errorf("no scenario emits %q, so its payload binding is unproven against real output; add one to realOutputScenarios", name)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(emitted)) {
		if !slices.Contains(declared, name) {
			t.Errorf("a real program emitted %q, which no Event constant declares", name)
		}
	}
}

// TestEventSchemaRejectsUnboundTypesAndPayloads shows the binding is enforced: an undeclared event type, a
// payload missing a required field and a payload with an undeclared field are all invalid.
func TestEventSchemaRejectsUnboundTypesAndPayloads(t *testing.T) {
	_, strict := publishedSchema(t, publishedEventSchema)
	line := func(eventType, payload string) []byte {
		return []byte(`{"object":"evo.event","schema_version":"` + wire.EventSchemaVersion +
			`","seq":1,"at":"2026-01-01T00:00:00Z","type":"` + eventType + `","payload":` + payload + `}`)
	}
	valid := line(wire.EventTaskDeclared, `{"name":"build"}`)
	if err := wireschema.Validate(strict, valid); err != nil {
		t.Fatalf("a well-formed task.declared is rejected: %v", err)
	}
	invalid := map[string][]byte{
		"an unknown event type":            line("task.exploded", `{"name":"build"}`),
		"task.declared without a name":     line(wire.EventTaskDeclared, `{}`),
		"task.declared with a stray field": line(wire.EventTaskDeclared, `{"name":"build","why":"x"}`),
		"effect.planned with a string qty": line(wire.EventEffectPlanned, `{"verb":"delete","object":"branch","quantity":"3"}`),
		"run.finished without an outcome":  line(wire.EventRunFinished, `{"exit_code":0}`),
		"task.started with a payload":      line(wire.EventTaskStarted, `{"name":"build"}`),
	}
	for name, doc := range invalid {
		if err := wireschema.Validate(strict, doc); err == nil {
			t.Errorf("%s validates; the schema must reject it", name)
		}
	}
}
