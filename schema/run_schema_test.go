package schema_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
	"github.com/zachbornheimer/evident-output/v2/testkit/wireschema"
)

const (
	runSchemaFile = "run.v3.json"
	contractFile  = "../CONTRACT.md"
)

// outcomeRow matches a row of CONTRACT.md's Outcome table: `Failed`, const, `Failed Outcome`.
var outcomeRow = regexp.MustCompile("(?m)^\\| `(\\w+)`\\s+\\| const \\| `\\w+ Outcome`")

// contractOutcomes are the six Outcomes CONTRACT.md §4 names, spelled as evo.run spells them.
func contractOutcomes(t *testing.T) []string {
	t.Helper()
	var outcomes []string
	for _, row := range outcomeRow.FindAllStringSubmatch(readFile(t, contractFile), -1) {
		outcomes = append(outcomes, strings.ToLower(row[1]))
	}
	if len(outcomes) != 6 {
		t.Fatalf("CONTRACT.md names %d Outcomes %v, want 6", len(outcomes), outcomes)
	}
	return outcomes
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(body)
}

// enumAt is the enum the schema's definition def gives property.
func enumAt(t *testing.T, schema map[string]any, def, property string) []string {
	t.Helper()
	node := schema
	if def != "" {
		node = schema["$defs"].(map[string]any)[def].(map[string]any)
	}
	values, ok := node["properties"].(map[string]any)[property].(map[string]any)["enum"].([]any)
	if !ok {
		t.Fatalf("%s %s.%s has no enum", runSchemaFile, def, property)
	}
	enum := make([]string, len(values))
	for i, value := range values {
		enum[i] = value.(string)
	}
	return enum
}

// TestRunSchemaEnumeratesStateAndOutcome pins CONTRACT.md §4: evo.run's
// outcome is one of the six Outcomes, a Task's state is an Outcome or
// not_started, and a collection's adds empty, so the published schema
// closes each set.
func TestRunSchemaEnumeratesStateAndOutcome(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal([]byte(readFile(t, runSchemaFile)), &schema); err != nil {
		t.Fatalf("parse %s: %v", runSchemaFile, err)
	}
	contract := readFile(t, contractFile)
	for _, name := range []string{"not_started", "empty"} {
		if !strings.Contains(contract, "`"+name+"`") {
			t.Errorf("CONTRACT.md does not name the state `%s`", name)
		}
	}
	outcomes := contractOutcomes(t)
	for _, c := range []struct {
		where string
		got   []string
		want  []string
	}{
		{"run outcome", enumAt(t, schema, "", "outcome"), outcomes},
		{"task outcome", enumAt(t, schema, "task", "outcome"), outcomes},
		{"task state", enumAt(t, schema, "task", "state"), append(slices.Clone(outcomes), "not_started")},
		{"collection state", enumAt(t, schema, "collection", "state"), append(slices.Clone(outcomes), "not_started", "empty")},
	} {
		if !slices.Equal(c.got, c.want) {
			t.Errorf("%s enum = %v, want %v", c.where, c.got, c.want)
		}
	}
}

// runDocument is a real evo.run document of a run that ends every way a
// Task can: Succeeded, and NotStarted because nothing Defined it.
func runDocument(t *testing.T) map[string]any {
	t.Helper()
	var stdout bytes.Buffer
	evo.Execute(context.Background(), evo.Config{
		Name: "schema", Output: evo.JSON,
		Facades: evo.Facades{Stdout: &stdout, Stderr: &bytes.Buffer{}, Stdin: strings.NewReader("")},
	}, func(_ context.Context, r *evo.Run) error {
		g := r.Group("group")
		g.Task("works").Define(func(context.Context) error { return nil })
		r.Task("undefined")
		return nil
	})
	var doc map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		t.Fatalf("stdout is not one evo.run document: %v\n%s", err, stdout.String())
	}
	return doc
}

// TestRunSchemaRejectsUnknownStateAndOutcome pins that a stock validator
// holds a real document to the closed sets: the document conforms as written,
// and each field fails when it names a value CONTRACT.md does not.
func TestRunSchemaRejectsUnknownStateAndOutcome(t *testing.T) {
	schema := []byte(readFile(t, runSchemaFile))
	check := func(doc map[string]any) error {
		body, err := json.Marshal(doc)
		if err != nil {
			t.Fatalf("marshal document: %v", err)
		}
		return wireschema.Validate(schema, body)
	}
	if err := check(runDocument(t)); err != nil {
		t.Fatalf("a real document does not conform to %s: %v", runSchemaFile, err)
	}
	for _, c := range []struct {
		name   string
		mutate func(doc map[string]any)
	}{
		{"run outcome", func(doc map[string]any) { doc["outcome"] = "partial" }},
		{"task state", func(doc map[string]any) { firstOf(doc, "tasks")["state"] = "incomplete" }},
		{"task outcome", func(doc map[string]any) { firstOf(doc, "tasks")["outcome"] = "done" }},
		{"collection state", func(doc map[string]any) { firstOf(doc, "collections")["state"] = "incomplete" }},
	} {
		t.Run(c.name, func(t *testing.T) {
			doc := runDocument(t)
			c.mutate(doc)
			if check(doc) == nil {
				t.Errorf("the schema accepts an unknown %s", c.name)
			}
		})
	}
}

// firstOf is the first entry of the document's data list.
func firstOf(doc map[string]any, list string) map[string]any {
	return doc["data"].(map[string]any)[list].([]any)[0].(map[string]any)
}
