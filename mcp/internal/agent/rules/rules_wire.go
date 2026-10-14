package rules

// wireRules is the EVO-WIRE-* family (spec §57): the machine-readable
// JSON/JSONL surface must stay the one sanctioned encoder, versioned, and
// never interleaved with human text on the same stream.
func wireRules() []Rule {
	return []Rule{
		{
			ID:        "EVO-WIRE-002",
			Category:  "WIRE",
			Severity:  SeverityError,
			Invariant: "a wire schema version constant is documented, and bumps in the same change that edits the wire fields",
			Why:       "Machine consumers parse the evo.run document and evo.event lines. Editing a wire struct's field set without bumping its schema version in the same change drops fields silently, and an undocumented version constant leaves a reviewer no record of why it moved.",
			BadCode: `const RunSchemaVersion = "3.0"

type RunDocument struct {
  SchemaVersion string   ` + "`json:\"schema_version\"`" + `
  Tasks         []string ` + "`json:\"tasks\"`" + `
}`,
			GoodCode: `// RunSchemaVersion is 3.0: tasks became a list of names (2.0 held objects).
const RunSchemaVersion = "3.0"`,
			Remediation:     "Document the version constant with what changed, and bump it (RunSchemaVersion in internal/project/wire/run.go, EventSchemaVersion in internal/project/wire/event.go) in the same change that adds, renames or removes a json property",
			RelatedGuidance: []string{"streams"},
			VerificationIDs: []string{"EVO-WIRE-002"},
			Since:           "1.0.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:        "EVO-WIRE-003",
			Category:  "WIRE",
			Severity:  SeverityError,
			Invariant: "stdout carries only the data payload; human text goes to Run.Text()",
			Why:       "A JSON document written to stdout alongside an ordinary human fmt.Print/Println on the same stream produces output no JSON or line parser can consume: the human line breaks json.Decoder mid-stream and corrupts every downstream jq or awk consumer. Run.ResultWriter() is the only writer to stdout in Human and Plain, and Run.Text() goes to stderr, so a pipe on stdout receives exactly the payload.",
			BadCode: `json.NewEncoder(r.ResultWriter()).Encode(doc)
fmt.Println("done") // human text lands in the payload stream`,
			GoodCode: `json.NewEncoder(r.ResultWriter()).Encode(doc)
fmt.Fprintln(r.Text(), "done") // human text goes to stderr`,
			BadOutput:       "{\"name\":\"zq\",...}\ndone",
			GoodOutput:      "{\"name\":\"zq\",...} // human text went to stderr",
			Remediation:     "Write human text with fmt.Fprintln(r.Text(), ...) and the data payload through r.ResultWriter(), so stdout carries only the payload",
			RelatedGuidance: []string{"streams"},
			VerificationIDs: []string{"EVO-WIRE-003"},
			Since:           "1.0.0",
			Certainty:       CertaintyHeuristic,
		},
	}
}

func init() { registerFamily(wireRules()) }
