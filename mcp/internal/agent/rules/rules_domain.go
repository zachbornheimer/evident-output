package rules

// domainRules is domain modelling: Task identity, taxonomy, evidence, and first-paint honesty (DOM/TAX/EV/FP).
func domainRules() []Rule {
	return []Rule{
		{
			ID:        "DOM-011",
			Category:  "DOM",
			Severity:  SeverityError,
			Invariant: "an expected, successfully evaluated answer such as a declined Confirm ends the run Refused, not Failed with a Go application error",
			Why:       "Refuse means evaluation succeeded and found a reason to stop. Returning errors.New for a declined question confuses agents and callers about refused (exit 1) vs failed (exit 2), and loses the non-interactive flag Confirm points the Next step at.",
			BadCode: `if !r.Confirm("delete 40 branches?", evo.Destructive(), evo.NonInteractive("--yes")) {
	return errors.New("declined") // wrong: a declined question is Refused, not a Failed application error
}`,
			GoodCode: `if !r.Confirm("delete 40 branches?", evo.Destructive(), evo.NonInteractive("--yes")) {
	return evo.Refuse("declined")
}`,
			Remediation:     "Return evo.Refuse(summary) when the user declines or nobody can answer; the Conclusion and Result.ExitCode follow it, and the application error stays nil",
			Exceptions:      []string{"I/O failures unrelated to the precondition"},
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"MCP-014", "DOM-011", "DOM-048"},
			Since:           "0.1.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:        "FP-001",
			Category:  "FP",
			Severity:  SeverityWarning,
			Invariant: "visible state paints within 100ms of process start",
			Why:       "A blank terminal for the first seconds of a run is indistinguishable from a hang; the user re-runs or ^C's healthy work. I/O ahead of evo.Main runs before anything is armed to paint.",
			BadCode: `func main() {
  cfg := loadConfig() // seconds of I/O, nothing on screen yet
  os.Exit(evo.Main(evo.Config{Name: cfg.Name}, run))
}`,
			GoodCode: `func main() { os.Exit(evo.Main(evo.Config{}, run)) }

func run(ctx context.Context, r *evo.Run) error {
  r.Task("load config").Define(func(ctx context.Context) error { return loadConfig() }) // the frame appears before any I/O
  return nil
}`,
			Remediation:     "Call evo.Main (or evo.Execute) as the first statement in main and move the I/O into the first Task's Define",
			RelatedGuidance: []string{"first-paint", "common-api"},
			VerificationIDs: []string{"FP-001"},
			Since:           "0.3.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:        "FP-002",
			Category:  "FP",
			Severity:  SeverityWarning,
			Invariant: "no I/O before the first entity is declared",
			Why:       "Declare-before-compute is what makes FP-001 achievable; I/O ahead of the first Task/Group/Sequence reintroduces the blank window.",
			BadCode: `func run(ctx context.Context, r *evo.Run) error {
  data, _ := os.ReadFile("config.toml") // I/O before any Task/Item/Sequence
  r.Task("scan")
  return nil
}`,
			GoodCode: `func run(ctx context.Context, r *evo.Run) error {
  scan := r.Task("scan")
  scan.Doing("reading config")
  data, _ := os.ReadFile("config.toml")
  return nil
}`,
			Remediation:     "Move the first Task/Group/Sequence declaration ahead of the first read/open/dial in the run body",
			RelatedGuidance: []string{"first-paint"},
			VerificationIDs: []string{"FP-002"},
			Since:           "0.3.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:        "FP-003",
			Category:  "FP",
			Severity:  SeverityWarning,
			Invariant: "phases advance on evidence; a stale phase is a defect",
			Why:       "A spinner whose text never changes is animation, not evidence — the user cannot tell slow from hung.",
			BadCode: `t := r.Task("salvage")
t.Doing("uploading")
cmd := exec.CommandContext(ctx, "git", "push")
cmd.Run() // silent for minutes; Doing text never refreshed`,
			GoodCode: `t := r.Task("salvage")
t.Doing("uploading")
cmd := exec.CommandContext(ctx, "git", "push")
cmd.Stdout = t.Writer() // the child's last line becomes the live text
cmd.Stderr = t.Writer()
cmd.Run()`,
			Remediation:     "Wire child output through Task.Writer, or advance Doing/Progress as evidence arrives; the built-in heartbeat covers the remaining silent case (>~10s) automatically",
			RelatedGuidance: []string{"first-paint", "tasks"},
			VerificationIDs: []string{"FP-003"},
			Since:           "0.3.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:        "TAX-001",
			Category:  "TAX",
			Severity:  SeverityWarning,
			Invariant: "reason partitions sum to the headline count; taxonomy is derived, never hand-assembled",
			Why:       "A bare \"skipped 6\" or a hand-built \"already mutated\" string can't be trusted — it can miscount, and the user can't tell why items were skipped.",
			BadCode:   `msg := fmt.Sprintf("skipped %d", n) // hand-assembled, no reason partition`,
			GoodCode: `branches := r.Group("branches")
branches.Task("main").Define(func(context.Context) error { return evo.Exclude("protected") })
branches.Task("feature/x").Define(func(context.Context) error { return evo.Exclude("dirty") })
// the item is the Task; evo derives each tally from its reason`,
			Remediation:     "Declare one Task per item and record its reason by returning evo.Exclude(reason) from its Define; let evo count, sum, and print the partition",
			RelatedGuidance: []string{"tasks"},
			VerificationIDs: []string{"TAX-001"},
			Since:           "0.6.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:              "FP-004",
			Category:        "FP",
			Severity:        SeverityWarning,
			Invariant:       "a Doing string names the domain object in motion, not a generic placeholder",
			Why:             "\"starting\"/\"working\"/\"running\"/\"please wait\" tells the user nothing changed since the last frame — the same illegible-spinner defect FP-003 covers for a silent subprocess.",
			BadCode:         `task.Doing("working")`,
			GoodCode:        `task.Doing("scanning ~/Developer/Personal/zq")`,
			Remediation:     "Name the object the task is currently acting on in the Doing string",
			RelatedGuidance: []string{"first-paint", "tasks"},
			VerificationIDs: []string{"FP-004"},
			Since:           "0.7.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:              "DOM-007",
			Category:        "DOM",
			Severity:        SeverityWarning,
			Invariant:       "returning evo.Refuse(summary, options...) builds exactly one Problem from ProblemOptions; do not hand-build a Problem literal for the common single-blocker case",
			Why:             "Hand-building a Problem{} literal duplicates what evo.Refuse(summary, evo.On(...), evo.Detail(...), ...) already does automatically and can drift from the sanitized/anonymous shape Refuse guarantees.",
			BadCode:         `return errors.New("disk full: " + err.Error()) // hand-assembled blocker text`,
			GoodCode:        `return evo.Refuse("disk full")`,
			Remediation:     "Return evo.Refuse(summary, options...) — evo.On/evo.Detail/evo.Code — instead of constructing a Problem by hand",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"DOM-007"},
			Since:           "0.1.0",
			Certainty:       CertaintyHeuristic,
			Detection:       DetectionGuidance, // no cheap detector: distinguishing a legitimate hand-built Problem from misuse needs call-site intent, not AST shape
		},
		{
			ID:        "DOM-017",
			Category:  "DOM",
			Severity:  SeverityWarning,
			Invariant: "Task.Progress(completed, total) stores absolute values, never a delta",
			Why:       "Hand-driving Progress from a loop index invites the exact bug it's meant to prevent — a re-run or retry that resets the counter reads as stuck, not incrementing.",
			BadCode: `for range items {
  t.Progress(1, total) // resets to 1 every call instead of incrementing
}`,
			GoodCode: `group := r.Group("items")
for _, item := range items {
  item := item
  group.Task(item).Define(func(context.Context) error { return work(item) })
}`,
			Remediation:     "Declare one named Task per item under Group/Sequence so each item is an atomic Task; do not hand-drive Progress from a loop index (Group.Each/Sequence.Each/Task.Each were removed in 1.0)",
			RelatedGuidance: []string{"tasks"},
			VerificationIDs: []string{"DOM-017"},
			Since:           "0.1.0",
			Certainty:       CertaintyHeuristic,
			Detection:       DetectionGuidance, // no cheap detector: a literal "1" argument is not distinguishable from a genuine absolute count by AST alone
		},
		{
			ID:              "DOM-018",
			Category:        "DOM",
			Severity:        SeverityWarning,
			Invariant:       "an error surfaces once per resolution, not as both the summary text and the Detail",
			Why:             "err.Error() as the summary alongside the same text in evo.Detail surfaces the same error twice; the 1.x spelling of the pair, evo.Cause(err), was removed in 1.1.",
			BadCode:         `return evo.Fail(err.Error(), evo.Detail(err.Error()))`,
			GoodCode:        `return evo.Fail("validate policy manifest", evo.Detail(err.Error()))`,
			Remediation:     `Give the summary a short context (what failed) and put err.Error() only in evo.Detail, or return fmt.Errorf inside Define`,
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"DOM-018"},
			Since:           "0.2.17",
			Certainty:       CertaintyDeterministic,
		},
		{
			ID:        "TAX-002",
			Category:  "TAX",
			Severity:  SeverityWarning,
			Invariant: "evo.Exclude's reason is a fixed string literal or a const/package-level var, never a computed expression",
			Why:       "An evo.Exclude reason built from a computed expression (Sprintf, Join, concatenation) opens one taxonomy bucket per distinct rendered value instead of one per classification (live instance: joining per-item counts into the reason text).",
			BadCode:   `return evo.Exclude(strings.Join(names, ", "))`,
			GoodCode: `group.Task(name).Define(func(context.Context) error {
  return evo.Exclude("protected") // the per-item detail is the Task's name, not the reason
})`,
			Remediation:     `Use a fixed string literal (or a package-level var) naming the classification; the dynamic detail is the item's own Task name`,
			RelatedGuidance: []string{"tasks"},
			VerificationIDs: []string{"TAX-002"},
			Since:           "0.2.17",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:        "DOM-019",
			Category:  "DOM",
			Severity:  SeverityWarning,
			Invariant: "a variable holding a Task is Defined before it is assigned another Task",
			Why:       "Assigning a second Task to a variable whose first Task never got Define drops the only handle to the first. It never runs: it ends NotStarted and the run concludes partial. DOM-021 misses it because the later Define on the same variable counts as settling the name.",
			BadCode: `t := r.Task("scan")
t.Doing("walking")
t = r.Task("build") // the "scan" Task never runs
t.Define(build)`,
			GoodCode: `t := r.Task("scan")
t.Define(scan)
t = r.Task("build")
t.Define(build)`,
			Remediation:     "Call Define on the first Task before reassigning the variable, or give the second declaration its own variable name",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"DOM-019"},
			Since:           "0.2.17",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:              "DOM-020",
			Category:        "DOM",
			Severity:        SeverityWarning,
			Invariant:       "usage/user mistakes resolve Refused (exit 1); evaluation failures resolve Failed (exit 2)",
			Why:             "Refuse and Fail carry different exit codes for a reason: a caller reading the exit code needs \"you did something wrong\" (1) and \"something broke while checking\" (2) to stay distinguishable. Routing a usage mistake through Fail reports a user error as a system failure.",
			BadCode:         `return evo.Fail("missing required --repo flag")`,
			GoodCode:        `return evo.Refuse("missing required --repo flag")`,
			Remediation:     "Route usage/user mistakes through Refuse, not Fail; reserve Fail for evaluation/execution failures",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"DOM-020"},
			Since:           "0.2.17",
			Certainty:       CertaintyHeuristic,
			// No cheap, honest static detector: telling "this Fail is a usage
			// mistake" from "this Fail is a genuine evaluation failure" needs
			// the caller's own domain judgment, not a source-level pattern.
			Detection: DetectionGuidance,
		},
		{
			ID:        "DOM-021",
			Category:  "DOM",
			Severity:  SeverityError,
			Invariant: "a Task a function declares is Defined or handed on before the function returns",
			Why:       "A Task only runs because of its Define. Wiring its Writer, adding Facts, Problems or a Verify gives it no work: it ends NotStarted, the run concludes partial with 'return from the Define callback of this Task: nil, or evo.Fail, evo.Refuse or evo.Exclude', and a child process run off-row never becomes the row's outcome.",
			BadCode: `t := r.Task("fetch")
cmd.Stdout = t.Writer()
return cmd.Run()`,
			GoodCode: `t := r.Task("fetch")
t.Define(func(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "git", "fetch")
	cmd.Stdout = t.Writer()
	return cmd.Run()
})`,
			Remediation:     "Run the work inside the Task's Define and return nil, evo.Fail, evo.Refuse or evo.Exclude",
			RelatedGuidance: []string{"common-api", "tasks"},
			VerificationIDs: []string{"DOM-021"},
			Since:           "1.1.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:              "TAX-003",
			Category:        "TAX",
			Severity:        SeverityWarning,
			Invariant:       "an Exclude reason names why the Task was excluded, not the verb it accompanies",
			Why:             "Excluded folds into one tally and its outcome already says the Task was excluded, so the reason is the only place left to say why. A reason that only restates the verb (`evo.Exclude(\"skipped\")`, zq cmd/zq-build/main.go:81) tells the user nothing they didn't already know from the glyph.",
			BadCode:         `return evo.Exclude("skipped") // restates the verb`,
			GoodCode:        `return evo.Exclude("protected branch") // names why`,
			Remediation:     "Name why the Task was excluded (the policy, the missing precondition, the protected target), not the outcome itself",
			RelatedGuidance: []string{"tasks"},
			VerificationIDs: []string{"TAX-003"},
			Since:           "0.4.7",
			Certainty:       CertaintyHeuristic,
		},
	}
}

func init() { registerFamily(domainRules()) }
