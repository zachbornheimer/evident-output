package rules

// wireRules is the EVO-WIRE-* family (spec §57): the machine-readable
// JSON/JSONL surface must stay the one sanctioned encoder, versioned, and
// never interleaved with human text on the same stream.
func wireRules() []Rule {
	return []Rule{
		{
			ID:              "EVO-WIRE-001",
			Category:        "WIRE",
			Severity:        "error",
			Invariant:       "the internal Snapshot/Result is never marshaled directly; the sanctioned wire encoder is the only JSON surface",
			Why:             "json.Marshal(out.Snapshot()) serializes internal field layout as if it were the public contract; a future internal-only field addition or rename then silently breaks every consumer, because nothing enforces schema_version or the documented JSONDocument shape.",
			BadCode:         `b, err := json.Marshal(out.Snapshot())`,
			GoodCode:        `b, err := render.EncodeJSON(out.Snapshot()) // or the CLI's --json flag, which already routes through this`,
			BadOutput:       `{"State":"Done","Tasks":[...]} // undocumented internal field names, no schema_version`,
			GoodOutput:      `{"schema_version":"0.4","output":{...},"conclusion":{...},"tasks":[...]}`,
			Remediation:     "Replace json.Marshal(snapshot)/json.NewEncoder(...).Encode(snapshot) with the sanctioned JSON encoder (render.EncodeJSON) that emits the versioned JSONDocument",
			RelatedGuidance: []string{"streams", "common-api"},
			VerificationIDs: []string{"EVO-WIRE-001"},
			Since:           "1.0.0",
			Certainty:       "deterministic",
		},
		{
			ID:        "EVO-WIRE-002",
			Category:  "WIRE",
			Severity:  "error",
			Invariant: "the legacy JSON/JSONL encoder's wire shape changes only alongside its own schema_version bump",
			Why:       "The 0.4 JSON series and 0.3 JSONL event series are pre-1.0 contracts that machine consumers already parse; editing an encoder's field set without bumping its schema_version constant in the same change is exactly the silent-drop class the 0.3→0.4 bump (warned/Warnings) fixed for the last edit — it must not regress.",
			BadCode: `// internal/render/json.go: add a field to JSONDocument/JSONTask
// with no change to JSONSchemaVersion in the same diff`,
			GoodCode: `// internal/render/json.go: add the field AND bump JSONSchemaVersion
// (or internal/core/event.go's EventSchemaVersion for JSONL) in the same diff,
// and record why in the const's doc comment`,
			Remediation:     "Bump JSONSchemaVersion (internal/render/json.go) or EventSchemaVersion (internal/core/event.go) in the same change that edits the legacy encoder's field set, and document the reason on the const",
			RelatedGuidance: []string{"streams"},
			VerificationIDs: []string{"EVO-WIRE-002"},
			Since:           "1.0.0",
			Certainty:       "heuristic",
			// No cheap, honest static detector: telling a schema-affecting
			// edit apart from a comment/refactor requires diffing the
			// encoder across two revisions, which a single-source review
			// call never sees. Guidance-only; enforced by review discipline
			// (goldens must stay byte-identical unless the version moves).
			Detection: "guidance",
		},
		{
			ID:        "EVO-WIRE-003",
			Category:  "WIRE",
			Severity:  "error",
			Invariant: "JSON/JSONL stdout is a machine payload stream; human presentation never shares it",
			Why:       "A JSON document or JSONL events written to stdout alongside an ordinary human fmt.Print/Println on the same stream produces output no JSON/line parser can consume — the human line breaks json.Decoder mid-stream and corrupts every downstream jq/awk consumer.",
			BadCode: `json.NewEncoder(os.Stdout).Encode(doc)
fmt.Println("done")`,
			GoodCode: `cfg.Format = evo.FormatData // JSON to Stdout, human presentation to Stderr
json.NewEncoder(os.Stdout).Encode(doc)`,
			BadOutput:       "{\"schema_version\":\"0.4\",...}\ndone",
			GoodOutput:      "{\"schema_version\":\"0.4\",...} // human presentation went to Stderr",
			Remediation:     "Route human presentation to Stderr (evo.FormatData) so Stdout carries only the JSON/JSONL payload",
			RelatedGuidance: []string{"streams"},
			VerificationIDs: []string{"EVO-WIRE-003"},
			Since:           "1.0.0",
			Certainty:       "heuristic",
		},
	}
}

func init() { registerFamily(wireRules()) }
