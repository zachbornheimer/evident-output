package rules

// provenanceRules is EVO-PROVENANCE-001 and EVO-PROVENANCE-002 (spec §57):
// the two rules guarding Inputs honesty and manifest-skip honesty.
// EVO-PROVENANCE-001 has a narrow, literal-only static detector
// (review.detectOmittedBasisPath): a string-literal path visibly read or
// passed as a literal Exec Arg in the same function that builds the
// Inputs, checked against that Inputs' own string literals — no
// variable is ever traced and no Inputs entry is ever invented, so it stays
// free of false positives while catching the exact shape its BadCode
// documents. EVO-PROVENANCE-002 has no such detector: distinguishing "a
// callback trusts a prior manifest entry alone" from a legitimate
// cached-but-reverified check needs call-site intent an AST shape cannot
// carry, so it stays Detection: DetectionGuidance and taught by example only
// (agent/review's regression suite proves it never *fabricates* a positive
// here; see review_evo_file_test.go).
func init() { registerFamily(provenanceRules()) }

func provenanceRules() []Rule {
	return []Rule{
		omittedBasisRule(),
		opaqueManifestSkipRule(),
	}
}

// omittedBasisRule is EVO-PROVENANCE-001: a generated artifact visibly
// reads a file or value that its own Inputs omit, so the tracked freshness
// no-ops even when that unlisted input has actually changed.
func omittedBasisRule() Rule {
	return Rule{
		ID:        "EVO-PROVENANCE-001",
		Category:  "EVO",
		Severity:  SeverityWarning,
		Invariant: "every file or value a generator visibly reads is listed in Inputs (or Values), or the operation's freshness claim is false",
		Why:       "evo.File/evo.Exec no-op when Inputs, Values, and outputs are all current. A generator that reads a config file, template, or environment value the call site never adds to Inputs or Values will silently skip re-running after that input changes — the operation reports itself fresh while its actual output is stale.",
		BadCode: `_, err := evo.Exec(ctx, evo.ExecSpec{
	Argv:    []string{"python3", "generate.py", "input.xlsx", "template.txt", "build/out.bin"},
	Inputs:  []string{"generate.py", "input.xlsx"}, // template.txt is read but omitted
	Outputs: []string{"build/out.bin"},
})
return err`,
		GoodCode: `if _, err := evo.Exec(ctx, evo.ExecSpec{
	Argv:    []string{"python3", "generate.py", "input.xlsx", "template.txt", "build/out.bin"},
	Inputs:  []string{"generate.py", "input.xlsx", "template.txt"},
	Outputs: []string{"build/out.bin"},
}); err != nil {
	return fmt.Errorf("generate build/out.bin: %w", err)
}
return nil`,
		Remediation:     "Add every file the generator actually reads to Inputs, and every value to Values; do not invent an Inputs entry the source does not justify (§58) — trace the generator's real inputs instead",
		RelatedGuidance: []string{"provenance", "evo-file-exec"},
		VerificationIDs: []string{"EVO-PROVENANCE-001"},
		Since:           "1.0.0",
		Certainty:       CertaintyHeuristic,
	}
}

// opaqueManifestSkipRule is EVO-PROVENANCE-002: code or docs claiming a
// prior manifest's provenance can skip a callback on a later run without
// current pre-definition proof (a live Verify) for that run.
func opaqueManifestSkipRule() Rule {
	return Rule{
		ID:        "EVO-PROVENANCE-002",
		Category:  "EVO",
		Severity:  SeverityWarning,
		Invariant: "a Task/operation may only report already-satisfied on the strength of proof this run observed, never on provenance an opaque callback merely recorded on some earlier run",
		Why:       "The manifest is a record of what a past run did, not evidence about the current filesystem/process state. An opaque callback (one whose body evo cannot inspect, e.g. a Verify-shaped func with no real current check) that skips work by trusting a prior manifest entry alone reports a false already-satisfied — it never re-observes the state it is claiming to confirm.",
		BadCode: `task.Verify(func(ctx context.Context) (bool, error) {
	// manifest says this ran before, so it's fine
	return manifest.HasEntry(name), nil
})`,
		GoodCode: `task.Verify(func(ctx context.Context) (bool, error) {
	return evo.File(ctx, evo.FileSpec{
		Path: path, Contents: contents, Mode: mode, Inputs: inputs,
	}) == nil, nil
})`,
		Remediation:     "Replace a manifest-only skip with a live pre-definition Verify (or a tracked evo.File/evo.Exec check) that proves the current state, not the recorded one",
		RelatedGuidance: []string{"provenance", "evidence-provenance"},
		VerificationIDs: []string{"EVO-PROVENANCE-002"},
		Since:           "1.0.0",
		Certainty:       CertaintyHeuristic,
		Detection:       DetectionGuidance, // no cheap detector: distinguishing "trusts the manifest alone" from a legitimate cached-but-reverified check needs call-site intent, not AST shape
	}
}
