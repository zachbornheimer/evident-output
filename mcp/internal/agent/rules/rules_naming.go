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
			Why:       "`Task(\"file integrity\")` (ZYS-838) names what the Task is about, not what it will determine; `Task(\"fix\")` (zq internal/app/app.go:80's a.task(\"fix\", ...) command family) reads as one row but really organizes several independently meaningful operations. Neither answers ZYS-838's own test: does the name alone tell the user what failed?",
			BadCode: `r.Task("file integrity").Define(checkIntegrity)
r.Task("fix").Define(fixAll)`,
			GoodCode: `r.Task("check file integrity").Define(checkIntegrity)

prep := r.Group("prepare staged files")
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
			MinDialect: "2.0.0",
			Category:   "API",
			Severity:   SeverityError,
			Invariant:  "a Group and a Sequence declared under one parent never share a name; Tasks have a name space of their own, but Group and Sequence share one",
			Why:        "`r.Group(\"build\")` followed by `r.Sequence(\"build\")` is a duplicate sibling: the second declaration is refused with ErrInvalidConfig, its handle is dead, the members declared under it never run, and the run ends Failed. A Task and a Group may share a name, so only Group against Sequence collides.",
			BadCode: `r.Group("build").Task("compile").Define(compile)
r.Sequence("build").Task("link").Define(link)`,
			GoodCode: `r.Group("build").Task("compile").Define(compile)
r.Sequence("package").Task("link").Define(link)`,
			Remediation:     "Give each Group and Sequence declared under one parent a name distinct from every other Group and Sequence there",
			RelatedGuidance: []string{"tasks"},
			VerificationIDs: []string{"API-047"},
			Since:           "2.0.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:        "API-048",
			Category:  "API",
			Severity:  SeveritySuggestion,
			Invariant: "a Group/Sequence Task referenced later (After, a second Define, ...) keeps its first handle in a variable; re-declaring by the same string literal is a duplicate sibling, not a get-or-create",
			Why:       "A second `Task(name)` call with an already-used name under the same parent is a duplicate sibling: it is refused with ErrInvalidConfig and fails the run rather than returning the earlier handle, so `prune.Task(\"branches\")` called again later to pass into After breaks the second Task instead of referencing the first. A typed var (...) block holds each handle once.",
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
