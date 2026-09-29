package rules

// concurrencyRules is concurrency and scheduler ordering (CON, EVO-DAG).
func concurrencyRules() []Rule {
	return []Rule{
		{
			ID:        "CON-001",
			Category:  "CON",
			Severity:  SeverityError,
			Invariant: "Partial is a completeness modifier; exit codes come from Outcome alone, and 130 is reserved for interruption",
			Detection: DetectionGuidance, // no cheap single-file detector: correct exit code use is unobservable from source (evo.Main hides the mapping)
			Why:       "Hand-mapping an exit code outside 0/1/2/130, or using 130 for something other than an actual interrupt, breaks the contract wrapping scripts and CI rely on.",
			BadCode: `if blocked {
  os.Exit(3) // hand-mapped code outside Outcome's 0/1/2/130
}
if partial {
  os.Exit(130) // Partial is not interruption
}`,
			GoodCode:        `os.Exit(evo.Main(run)) // code derives from Outcome alone: 0 OK / 1 Blocked / 2 Failed / 130 Cancelled; Partial is a completeness modifier only`,
			Remediation:     "Let evo.Main derive the exit code from Outcome (caller writes os.Exit(evo.Main(run))), or an Isolated instance's Output.Run which returns the same code (evo.MainWith was removed in 1.0); never hand-map an int, and never use 130 outside real interruption",
			RelatedGuidance: []string{"streams"},
			VerificationIDs: []string{"CON-001"},
			Since:           "0.6.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:        "CON-002",
			Category:  "CON",
			Severity:  SeverityWarning,
			Invariant: "a failure summary is printed once, from Conclusion, not hand-assembled from a collected list",
			Why:       "fmt/out.Print(strings.Join(failures, ...)) duplicates the exact summary Conclusion already owns and can drift from the glyphs/exit code the ledger shows.",
			BadCode:   `out.Println(strings.Join(failures, "\n")) // duplicates Conclusion`,
			GoodCode: `for _, f := range failures {
  out.Task(f.Name).Fail(f.Reason)
}
// Conclusion renders the one summary; add Next(evo.Label(...)) for guidance`,
			Remediation:     "Resolve each failure on its own Item/Task and let Conclusion summarize; use Next for follow-up guidance",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"CON-002"},
			Since:           "0.7.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:         "EVO-DAG-001",
			MinDialect: "1.0.0",
			Category:   "EVO",
			Severity:   SeverityWarning,
			Invariant:  "application code does not create a goroutine merely to make Evo Tasks run in parallel; Group already schedules independent children concurrently",
			Why:        "a goroutine wrapping a call that itself submits work to Evo's scheduler (.Define) is redundant parallelism the scheduler already provides, and it forfeits Evo's own concurrency limits and cancellation handling (spec §4).",
			BadCode: `for _, pkg := range pkgs {
  go func(pkg Package) {
    task := group.Task(pkg.Name)
    task.Define(func(ctx context.Context) error { return install(ctx, pkg) })
  }(pkg)
}`,
			GoodCode: `for _, pkg := range pkgs {
  task := group.Task(pkg.Name)
  task.Define(func(ctx context.Context) error { return install(ctx, pkg) })
}`,
			Remediation:     "Delete the goroutine and call task.Define(...) directly; Group already runs independent Tasks concurrently",
			RelatedGuidance: []string{"tasks", "common-api"},
			VerificationIDs: []string{"EVO-DAG-001"},
			Since:           "1.0.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:         "EVO-DAG-002",
			MinDialect: "1.0.0",
			Category:   "EVO",
			Severity:   SeverityWarning,
			Invariant:  "a hand-chained sequence of .After(...) calls is expressed as an evo.Sequence instead",
			Why:        "After is the exceptional explicit DAG edge (spec §6); a chain of two or more .After(...) calls reproduces exactly the linear ordering Sequence already gives its children automatically, with no exceptional edge left to justify hand-wiring it.",
			BadCode: `register := seq.Task("register")
register.After(write)
start := seq.Task("start")
start.After(register)`,
			GoodCode: `seq := evo.Sequence("launch agent")
write := seq.Task("write plist")
register := seq.Task("register")
start := seq.Task("start")`,
			Remediation:     "Replace the chained .After(...) calls with one evo.Sequence and declare each Task as seq.Task(...) in order",
			RelatedGuidance: []string{"tasks", "common-api"},
			VerificationIDs: []string{"EVO-DAG-002"},
			Since:           "1.0.0",
			Certainty:       CertaintyDeterministic,
		},
		{
			ID:         "EVO-DAG-003",
			MinDialect: "1.0.0",
			Category:   "EVO",
			Severity:   SeverityWarning,
			Invariant:  "a visible producer/consumer resource relationship between two Tasks has an explicit first-run scheduler edge (Sequence or After)",
			Why:        "known-producer freshness barriers only delay Evidence evaluation once a manifest already exists; on a first run there is no prior manifest to consult, so an unordered producer/consumer pair can race (spec §11.5, §47's \"first-run producer/consumer ordering still requires Sequence/After\").",
			BadCode: `producer.Define(func(ctx context.Context) error {
  return evo.File(ctx, evo.FileSpec{Path: "config.json", Contents: cfg})
})
consumer.Define(func(ctx context.Context) error {
  _, err := os.ReadFile("config.json")
  return err
})`,
			GoodCode: `consumer.After(producer)
consumer.Define(func(ctx context.Context) error {
  _, err := os.ReadFile("config.json")
  return err
})`,
			Remediation:     "Add consumer.After(producer), or declare both Tasks under one evo.Sequence so the producer always runs first",
			RelatedGuidance: []string{"tasks", "common-api"},
			VerificationIDs: []string{"EVO-DAG-003"},
			Since:           "1.0.0",
			Certainty:       CertaintyHeuristic,
		},
	}
}

func init() { registerFamily(concurrencyRules()) }
