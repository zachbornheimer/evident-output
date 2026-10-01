package rules

// effectRules is the evo.Effect family: an Effect callback must do the
// mutation it names (API-042), name one object (API-043), and never write
// file state that evo.File owns (API-057).
func effectRules() []Rule {
	return []Rule{
		{
			ID:        "API-042",
			Category:  "API",
			Severity:  SeverityError,
			Invariant: "an evo.Effect callback does the mutation; nil or a no-op callback is theater over work that ran elsewhere",
			Why:       "A nil Effect callback (zq README.md:39's old Create(\"module\", nil)) and one that only returns installedPythonModuleCount(name, n) (zq setup_python.go:172-181, where the named func only validates a count) both let the bulk work already run outside the callback, then hand Effect an empty gesture the ledger records as a real mutation.",
			BadCode: `spec := evo.EffectSpec{Verb: evo.EffectCreate, Object: "module", Quantity: n}
evo.Effect(ctx, spec, nil)
evo.Effect(ctx, spec, func(context.Context) error { return installedPythonModuleCount(name, n) })`,
			GoodCode: `spec := evo.EffectSpec{Verb: evo.EffectCreate, Object: "module", Quantity: n}
evo.Effect(ctx, spec, func(ctx context.Context) error {
  return invokeUV(ctx, root, packages)
})`,
			Remediation:     "Move the real mutation into the Effect callback. When the work already ran elsewhere and only information remains, report it with task.Fact instead of an Effect",
			RelatedGuidance: []string{"tasks"},
			VerificationIDs: []string{"API-042"},
			Since:           "0.4.7",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:              "API-043",
			Category:        "API",
			Severity:        SeverityWarning,
			Invariant:       "an EffectSpec.Object literal names the singular; evo pluralizes it from Quantity",
			Why:             "`EffectSpec{Verb: EffectDelete, Object: \"worktrees\", Quantity: 1}` renders \"deleted 1 worktrees\" (zq axis-14 P17) because Pluralize treats an already-plural literal as unchanged; Object must stay singular so pluralization has one job.",
			BadCode:         `evo.EffectSpec{Verb: evo.EffectDelete, Object: "worktrees", Quantity: 1}`,
			GoodCode:        `evo.EffectSpec{Verb: evo.EffectDelete, Object: "worktree", Quantity: 1}`,
			Remediation:     "Pass the singular noun as EffectSpec.Object; let Quantity drive pluralization",
			RelatedGuidance: []string{"tasks"},
			VerificationIDs: []string{"API-043"},
			Since:           "0.4.7",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:         "API-057",
			MinDialect: "1.1.0",
			Category:   "API",
			Severity:   SeverityError,
			Invariant:  "an evo.Effect callback never mutates the filesystem directly; Effect is the opaque-mutation escape hatch for work Evo cannot model declaratively (a git ref, a remote API call, a database row), and file-backed state always routes through evo.File",
			Why:        "evo.Write and its sibling TaskHandle mutation verbs were removed outright in 1.1 precisely because a generic write-shaped callback silently loses file resource identity, Basis, stale-write protection, desired-state comparison, AlreadySatisfied, and verification (ZYS-851). evo.Effect is the reduced opaque-mutation primitive that replaced them; a caller who reaches for it to write a file recreates the exact footgun 1.1 removed, just one layer deeper, and the object string alone (\"config file\", \"manifest.json\") is not reliable evidence — only a known filesystem mutator call inside the callback is (ZYS-851 Decisions, 2026-09-23). evo.File is the route for file-backed state, including writes derived from an existing file's own contents; a write derived from a unified diff instead goes through evo.Patch (API-058/API-059, ZYS-934/ZYS-935/ZYS-841/ZYS-1382) — neither is a second write API layered under Effect.",
			BadCode: `task.Define(func(ctx context.Context) error {
  return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectUpdate, Object: "config file", Quantity: 1}, func(context.Context) error {
    return os.WriteFile(path, contents, 0o644)
  })
})`,
			GoodCode: `task.Define(func(ctx context.Context) error {
  return evo.File{Path: path, Content: evo.Bytes(contents), Mode: 0o644}.Write(ctx)
})`,
			Remediation:     "Delete the evo.Effect wrapping the file write; call evo.File{...}.Write(ctx) directly — read the existing contents first if the new contents derive from them, then pass the derived result as File.Content",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"API-057"},
			Since:           "1.1.0",
			Certainty:       CertaintyHeuristic,
		},
	}
}

func init() { registerFamily(effectRules()) }
