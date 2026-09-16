package rules

// provenanceRules is EVO-PROVENANCE-001 and EVO-PROVENANCE-002 (spec §57):
// the two rules guarding Basis honesty and manifest-skip honesty. Neither
// has a cheap static detector that stays free of false positives — reading
// an omitted file, or skipping on stale provenance, is a semantic judgment
// about a generator's actual behavior, not an AST shape — so both are
// Detection: "guidance" and taught by example only (agent/review's
// regression suite instead proves it never *fabricates* a positive here;
// see review_evo_provenance_test.go).
func provenanceRules() []Rule {
	return []Rule{
		omittedBasisRule(),
		opaqueManifestSkipRule(),
	}
}

// omittedBasisRule is EVO-PROVENANCE-001: a generated artifact visibly
// reads a file or value that its own Basis omits, so the tracked freshness
// no-ops even when that unlisted input has actually changed.
func omittedBasisRule() Rule {
	return Rule{
		ID:        "EVO-PROVENANCE-001",
		Category:  "EVO",
		Severity:  "warning",
		Invariant: "every file or value a generator visibly reads is listed in Basis, or the operation's freshness claim is false",
		Why:       "evo.File/evo.Exec no-op when Basis, identity, and outputs are all current. A generator that reads a config file, template, or environment value the call site never adds to Basis will silently skip re-running after that input changes — the operation reports itself fresh while its actual output is stale.",
		BadCode: `return evo.Exec(ctx, evo.ExecSpec{
	Executable: "python3",
	Args:       []string{"generate.py", "input.xlsx", "template.txt", "build/out.bin"},
	Basis:      []evo.Fingerprint{evo.FSPath("input.xlsx")}, // template.txt is read but omitted
	Outputs:    []string{"build/out.bin"},
})`,
		GoodCode: `return evo.Exec(ctx, evo.ExecSpec{
	Executable: "python3",
	Args:       []string{"generate.py", "input.xlsx", "template.txt", "build/out.bin"},
	Basis: []evo.Fingerprint{
		evo.FSPath("input.xlsx"),
		evo.FSPath("template.txt"),
	},
	Outputs: []string{"build/out.bin"},
})`,
		Remediation:     "Add every file/value the generator actually reads to Basis; do not invent a Basis entry the source does not justify (§58) — trace the generator's real inputs instead",
		RelatedGuidance: []string{"provenance", "evo-exec", "evo-file"},
		VerificationIDs: []string{"EVO-PROVENANCE-001"},
		Since:           "1.0.0",
		Certainty:       "heuristic",
		Detection:       "guidance", // no cheap detector: proving a generator "visibly reads" a path means tracing an external process/script, not this file's AST
	}
}

// opaqueManifestSkipRule is EVO-PROVENANCE-002: code or docs claiming a
// prior manifest's provenance can skip a callback on a later run without
// current pre-definition proof (a live Verify) for that run.
func opaqueManifestSkipRule() Rule {
	return Rule{
		ID:        "EVO-PROVENANCE-002",
		Category:  "EVO",
		Severity:  "warning",
		Invariant: "a Task/operation may only report already-satisfied on the strength of proof this run observed, never on provenance an opaque callback merely recorded on some earlier run",
		Why:       "The manifest is a record of what a past run did, not evidence about the current filesystem/process state. An opaque callback (one whose body evo cannot inspect, e.g. a Verify-shaped func with no real current check) that skips work by trusting a prior manifest entry alone reports a false already-satisfied — it never re-observes the state it is claiming to confirm.",
		BadCode: `task.Verify(func(ctx context.Context) (bool, error) {
	// manifest says this ran before, so it's fine
	return manifest.HasEntry(task.Name()), nil
})`,
		GoodCode: `task.Verify(func(ctx context.Context) (bool, error) {
	return evo.File(ctx, evo.FileSpec{
		Path: path, Contents: contents, Mode: mode, Basis: basis,
	}) == nil, nil
})`,
		Remediation:     "Replace a manifest-only skip with a live pre-definition Verify (or a tracked evo.File/evo.Exec check) that proves the current state, not the recorded one",
		RelatedGuidance: []string{"provenance", "evidence"},
		VerificationIDs: []string{"EVO-PROVENANCE-002"},
		Since:           "1.0.0",
		Certainty:       "heuristic",
		Detection:       "guidance", // no cheap detector: distinguishing "trusts the manifest alone" from a legitimate cached-but-reverified check needs call-site intent, not AST shape
	}
}
