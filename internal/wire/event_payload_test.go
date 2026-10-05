package wire

import (
	"encoding/json"
	"reflect"
	"testing"
)

// fullProblemDoc sets every ProblemDoc field, so a field EventPayload
// forgets shows up as a missing key below.
var fullProblemDoc = ProblemDoc{
	Code: "E_BUILD", Message: "build failed", Subject: "main.go",
	Detail: "compiler error", CaptureTail: "undefined: x",
	Count: 2, Unit: "error",
	Location: &LocationDoc{Path: "main.go", Line: 12, Column: 3},
	Remedies: []ActionDoc{{Label: "rerun", Command: &CommandDoc{Executable: "go", Args: []string{"build"}}}},
}

var fullVerificationDoc = VerificationDoc{
	Name: "permissions", Status: VerificationError,
	Facts: []FactDoc{{Name: "error", Value: "operation not permitted"}},
}

// asJSONObject encodes v and decodes it back as a generic JSON object, the
// shape a JSONL consumer sees.
func asJSONObject(t *testing.T, v any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %T: %v", v, err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal %T: %v", v, err)
	}
	return out
}

// TestEventPayload_EncodesAsItsDoc proves each §38 payload carries exactly
// the fields and values its evo.run doc encodes, with ProblemDoc's
// "message" renamed to JSONL's "summary" and nothing else changed.
func TestEventPayload_EncodesAsItsDoc(t *testing.T) {
	problemWant := asJSONObject(t, fullProblemDoc)
	problemWant[problemEventSummaryKey] = problemWant["message"]
	delete(problemWant, "message")

	cases := []struct {
		name    string
		payload map[string]any
		want    map[string]any
	}{
		{"problem", fullProblemDoc.EventPayload(), problemWant},
		{"problem without optional fields", ProblemDoc{Message: "m"}.EventPayload(), map[string]any{problemEventSummaryKey: "m"}},
		{"verification", fullVerificationDoc.EventPayload(), asJSONObject(t, fullVerificationDoc)},
		{"verification without facts", VerificationDoc{Name: "n", Status: VerificationSatisfied}.EventPayload(),
			asJSONObject(t, VerificationDoc{Name: "n", Status: VerificationSatisfied})},
		{"fact", fullVerificationDoc.Facts[0].EventPayload(), asJSONObject(t, fullVerificationDoc.Facts[0])},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := asJSONObject(t, tc.payload); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("payload encodes as\n%v\nwant\n%v", got, tc.want)
			}
		})
	}
}

// TestEventPayload_FixturesSetEveryField keeps the fixtures above honest:
// a doc field they leave zero would pass TestEventPayload_EncodesAsItsDoc
// even when EventPayload never emits it.
func TestEventPayload_FixturesSetEveryField(t *testing.T) {
	for _, doc := range []any{fullProblemDoc, fullVerificationDoc, fullVerificationDoc.Facts[0]} {
		v := reflect.ValueOf(doc)
		for i := range v.NumField() {
			if v.Field(i).IsZero() {
				t.Errorf("%T fixture leaves %s zero; set it so its payload key is checked", doc, v.Type().Field(i).Name)
			}
		}
	}
}
