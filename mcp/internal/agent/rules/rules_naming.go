package rules

// namingRules is the Task-naming family (1.1): a Task name says what it
// does, not only its subject or container (API-045), and sibling names
// stay unique across kinds and redeclarations (API-047, API-048).
func namingRules() []Rule {
	return []Rule{
		{
			ID:        "API-045",
			Category:  "API",
			Severity:  SeverityWarning,
			Invariant: "a Task names one independently schedulable promise whose outcome is independently meaningful to the user, not a subject label or a container wearing one Task's clothes",
			Why:       "`Task(\"file integrity\")` (ZYS-838, also this codebase's own FP-006 fixture) names what the Task is about, not what it will determine; `Task(\"fix\")` (zq internal/app/app.go:80's a.task(\"fix\", ...) command family) reads as one row but really organizes several independently meaningful operations. Neither answers ZYS-838's own test: does the name alone tell the user what failed?",
			BadCode: `out.Task("file integrity").Define(checkIntegrity)
out.Task("fix").Define(fixAll)`,
			GoodCode: `out.Task("check file integrity").Define(checkIntegrity)

prep := out.Group("prepare staged files")
prep.Task("format Python").Define(formatPython)
prep.Task("stabilize Go source").Define(stabilizeGo)`,
			Remediation:     "Rename a subject-only Task to verb+object; replace a generic container Task with a Group/Sequence whose children are the independently meaningful Tasks",
			RelatedGuidance: []string{"tasks"},
			VerificationIDs: []string{"API-045"},
			Since:           "1.1.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:         "API-047",
			MinDialect: "1.0.0",
			Category:   "API",
			Severity:   SeverityError,
			Invariant:  "a Task/Group/Sequence's default §3.1 identity folds its kind into the stable key (kind:parentKey/name); a sibling name reused across different kinds under one parent is two distinct runtime identities sharing one visible display name",
			Why:        "`out.Task(\"build\")` and `out.Group(\"build\")` never collide at runtime — failDuplicateSiblingLocked's dedup check only compares within one kind's own name index — so both declare successfully and render as two rows a reader cannot tell apart by name alone, even though provenance/manifest lookups by display name now resolve ambiguously between them.",
			BadCode: `out.Task("build")
out.Group("build")`,
			GoodCode: `out.Task("build")
out.Group("build assets")`,
			Remediation:     "Give each Task/Group/Sequence declared under one parent a name distinct from every sibling, regardless of kind — not only from siblings of its own kind",
			RelatedGuidance: []string{"tasks"},
			VerificationIDs: []string{"API-047"},
			Since:           "1.1.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:        "API-048",
			Category:  "API",
			Severity:  SeveritySuggestion,
			Invariant: "a Group/Sequence Task referenced later (After, a second Define, ...) keeps its first handle in a variable; re-declaring by the same string literal is a duplicate sibling, not a get-or-create",
			Why:       "GroupHandle.Task(name)'s second call with an already-used name fails as a duplicate sibling (declareGroupTask, §3.1) rather than returning the earlier handle, so `prune.Task(\"branches\")` called again later to pass into After silently breaks the second Task instead of referencing the first. The product contract's own zq prune fixture (§18/§21) extracts these into a typed var (...) block instead.",
			BadCode: `prune.Task("branches").Define(func(ctx context.Context) error { return nil })
prune.Task("remote-tracking").
  After(prune.Task("branches")). // re-declares "branches"; fails as a duplicate sibling
  Define(func(ctx context.Context) error { return nil })`,
			GoodCode: `var (
  branches = prune.Task("branches")
  remote   = prune.Task("remote-tracking")
)
branches.Define(func(ctx context.Context) error { return nil })
remote.After(branches).Define(func(ctx context.Context) error { return nil })`,
			Remediation:     "Keep the first Task(name) handle in a typed variable (a var (...) block when there are several) and reuse it for the later reference; do not require this for a Task named only once",
			RelatedGuidance: []string{"tasks", "common-api"},
			VerificationIDs: []string{"API-048"},
			Since:           "1.1.0",
			Certainty:       CertaintyHeuristic,
		},
	}
}

func init() { registerFamily(namingRules()) }
