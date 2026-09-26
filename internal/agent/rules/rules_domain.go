package rules

// domainRules is domain modelling: Task identity, taxonomy, evidence, and first-paint honesty (DOM/TAX/EV/FP).
func domainRules() []Rule {
	return []Rule{
		{
			ID:        "DOM-011",
			Category:  "DOM",
			Severity:  SeverityError,
			Invariant: "expected blocked items are presentation outcomes, not Go application errors",
			Why:       "Block means evaluation succeeded and found a blocker. Returning errors.New after Block confuses agents and callers about failure vs blocked.",
			BadCode: `it := out.Task("working tree")
it.Block("dirty")
return errors.New("dirty") // wrong: blocked is not an app error`,
			GoodCode: `it := out.Task("working tree")
it.Block("dirty")
if err := out.Finish(); err != nil {
  return err // misuse only
}
os.Exit(out.Conclusion().ExitCode) // or return nil to caller that checks ExitCode`,
			Remediation:     "After Block, Finish and use Conclusion.ExitCode; return nil for successful evaluation that found a blocker",
			Exceptions:      []string{"wrapping Finish misuse errors", "I/O failures unrelated to Block"},
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
			Why:       "A blank terminal for the first seconds of a run is indistinguishable from a hang; the user re-runs or ^C's healthy work.",
			BadCode: `func main() {
  cfg := loadConfig() // seconds of I/O, nothing on screen yet
  evo.Init(evo.Config{Title: "tool"}) // too late — loadConfig already ran
  evo.Task("scan")
}`,
			GoodCode: `func main() {
  evo.Init(evo.Config{Title: "tool"}) // arms first paint before any I/O
  evo.Task("scan").Doing("reading config")
  cfg := loadConfig()
}`,
			Remediation:     "Call evo.Init as the first statement in main and declare the first Task/Item/Sequence before any I/O",
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
			Why:       "Declare-before-compute is what makes FP-001 achievable; I/O ahead of the first Task/Item/Sequence reintroduces the blank window.",
			BadCode: `func main() {
  evo.Init(evo.Config{Title: "tool"})
  data, _ := os.ReadFile("config.toml") // I/O before any Task/Item/Sequence
  evo.Task("scan")
}`,
			GoodCode: `func main() {
  evo.Init(evo.Config{Title: "tool"})
  scan := evo.Task("scan")
  scan.Doing("reading config")
  data, _ := os.ReadFile("config.toml")
}`,
			Remediation:     "Move the first Task/Item/Sequence declaration ahead of the first read/open/dial in main or run",
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
			BadCode: `t := evo.Task("salvage")
t.Doing("uploading")
run.Run(ctx, "git", args, io.Discard) // silent for minutes; doing-text never refreshed`,
			GoodCode: `t := evo.Task("salvage")
t.Doing("uploading")
run.Run(ctx, "git", args, t.Writer()) // last child line becomes the live doing-text
// or stream discovery into Progress as totals become known`,
			Remediation:     "Wire child output through Task.Writer, or advance Doing/Progress as evidence arrives; the built-in heartbeat covers the remaining silent case (>~10s) automatically",
			RelatedGuidance: []string{"first-paint", "tasks"},
			VerificationIDs: []string{"FP-003"},
			Since:           "0.3.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:        "DOM-014",
			Category:  "DOM",
			Severity:  SeverityError,
			Invariant: "Detail is a user-visible string; a diagnostic error's text is folded into the Block/Fail summary, not passed as Detail(err)",
			Why:       "Detail(err) exposes error internals as UI copy. Fold the error's text into the summary string instead (Blockf/Failf, the %w-wrapping siblings that once did this in one line, were removed in the owner vocabulary freeze, 2026-09-25).",
			BadCode:   `it.Block("dirty", evo.Detail(err))`,
			GoodCode: `it.Block("dirty: " + err.Error())
return err`,
			Remediation:     `Replace Detail(err) with the error's text folded into the summary, e.g. it.Block("dirty: " + err.Error()); reserve Detail for user-visible strings`,
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"DOM-014"},
			Since:           "0.1.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:        "TAX-001",
			Category:  "TAX",
			Severity:  SeverityWarning,
			Invariant: "reason partitions sum to the headline count; taxonomy is derived, never hand-assembled",
			Why:       "A bare \"skipped 6\" or a hand-built \"already mutated\" string can't be trusted — it can miscount, and the user can't tell why items were skipped.",
			BadCode:   `msg := fmt.Sprintf("skipped %d", n) // hand-assembled, no reason partition`,
			GoodCode: `branches := out.Group("branches")
branches.Task("main").Skipped(evo.Reason("protected"))
branches.Task("feature/x").Skipped(evo.Reason("dirty"))
// the item is the Task; evo derives each tally from its Reason`,
			Remediation:     "Declare one Task per item and record its reason via task.Skipped (or task.Fact for a kept item); let evo count, sum, and print the partition",
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
			ID:        "EV-001",
			Category:  "EV",
			Severity:  SeverityWarning,
			Invariant: "a failure summary does not manually embed the retained captured text",
			Why:       "task.Fail(\"install failed: \" + capture.Text()) folds the retained output straight into the summary the row already shows; auto-attach then renders the exact same text a second time underneath it (user-13-problems.md Problem 7: \"execution owns capture, callers provide context\").",
			BadCode:   `task.Fail("install failed: " + capture.Text())`,
			GoodCode: `cmd.Stdout = task.Writer() // retained and auto-attached on failure
cmd.Stderr = task.Writer()
if err := cmd.Run(); err != nil {
  task.Fail("install dependencies: " + err.Error())
  return err
}`,
			Remediation:     "Pass context via the error's own text instead of interpolating capture.Text() into the summary — Fail/Block auto-attach the retained tail as its own detail line",
			RelatedGuidance: []string{"streams"},
			VerificationIDs: []string{"EV-001"},
			Since:           "0.4.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:        "DOM-006",
			Category:  "DOM",
			Severity:  SeverityWarning,
			Invariant: "Item.OK/Block is a direct legal terminal transition; explicit Start is not required",
			Why:       "An Item is pending until it resolves; calling Start first is redundant ceremony and risks a spinner flash for a transition that finishes instantly.",
			BadCode: `it := out.Task("disk space")
it.Start()
it.Define(checkDiskSpace)`,
			GoodCode: `it := out.Task("disk space")
it.Define(checkDiskSpace)`,
			Remediation:     "Call Define (or Problem/Block/Fail) directly; remove the explicit Start call",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"DOM-006"},
			Since:           "0.1.0",
			Certainty:       CertaintyDeterministic,
		},
		{
			ID:              "DOM-007",
			Category:        "DOM",
			Severity:        SeverityWarning,
			Invariant:       "Task.Block(summary, options...) builds exactly one Problem from ProblemOptions; do not hand-build a Problem literal for the common single-blocker case",
			Why:             "Hand-building a Problem{} literal duplicates what Block(summary, evo.On(...), evo.Count(...), ...) already does automatically and can drift from the sanitized/anonymous shape Block guarantees.",
			BadCode:         `task.Block("disk full", evo.Cause(fmt.Errorf("hand-built: %w", err)))`,
			GoodCode:        `task.Block("disk full")`,
			Remediation:     "Use Block(summary, options...) — evo.On/evo.Count/evo.Detail — instead of constructing a Problem by hand",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"DOM-007"},
			Since:           "0.1.0",
			Certainty:       CertaintyHeuristic,
			Detection:       DetectionGuidance, // no cheap detector: distinguishing a legitimate hand-built Problem from misuse needs call-site intent, not AST shape
		},
		{
			ID:        "DOM-016",
			Category:  "DOM",
			Severity:  SeverityWarning,
			Invariant: "Task.Doing without a prior Start activates the task directly into running, indeterminate state",
			Why:       "Requiring Start before Doing is the same redundant ceremony API-006 already forbids for Define; Doing alone carries enough information to activate the task.",
			BadCode: `t := out.Task("scan")
t.Start()
t.Doing("walking")`,
			GoodCode: `t := out.Task("scan")
t.Doing("walking")`,
			Remediation:     "Call Doing directly; do not call Start first — see API-006 for the general no-Start-needed rule",
			RelatedGuidance: []string{"tasks", "common-api"},
			VerificationIDs: []string{"DOM-016"},
			Since:           "0.1.0",
			Certainty:       CertaintyHeuristic,
			Detection:       DetectionGuidance, // API-006 already carries the detector for the shared Start-then-X shape; DOM-016 documents the resulting state
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
			GoodCode: `group := evo.Group("items")
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
			ID:        "DOM-018",
			Category:  "DOM",
			Severity:  SeverityWarning,
			Invariant: "an error surfaces once per resolution, not as both the summary text and evo.Cause",
			Why:       "err.Error() as the summary alongside evo.Cause(err) surfaces the same error twice — and since Fail/Block are statement-form, evo.Cause no longer affects the returned error at all, so the two are now the identical dead-and-live text.",
			BadCode:   `task.Fail(err.Error(), evo.Cause(err))`,
			GoodCode: `task.Fail("validate policy manifest: " + err.Error())
return err`,
			Remediation:     `Replace the err.Error()+evo.Cause(err) pair with one Fail/Block call whose summary already folds in the error text, then return the error separately (Failf/Blockf, the once-canonical %w-wrapping siblings, were removed in the owner vocabulary freeze, 2026-09-25)`,
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"DOM-018"},
			Since:           "0.2.17",
			Certainty:       CertaintyDeterministic,
		},
		{
			ID:              "TAX-002",
			Category:        "TAX",
			Severity:        SeverityWarning,
			Invariant:       "evo.Reason's argument is a fixed string literal or a const/package-level var, never a computed expression",
			Why:             "evo.Reason built from a computed expression (Sprintf, Join, concatenation) opens one taxonomy bucket per distinct rendered value instead of one per classification (live instance: joining per-item counts into the reason text).",
			BadCode:         `task.Skipped(evo.Reason(strings.Join(names, ", ")), name)`,
			GoodCode:        `group.Task(name).Skipped(evo.Reason("protected")) // the per-item detail is the Task's name, not the reason`,
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
			Invariant: "a live Task/Item handle variable is resolved before it is reassigned to a new declaration",
			Why:       "Reassigning a variable from a new Task/Item declaration before the previous handle it held was resolved orphans the earlier row Running forever — a double row hiding under one variable name.",
			BadCode: `t := out.Task("scan")
t.Doing("walking")
t = out.Task("build") // the "scan" row never resolves`,
			GoodCode: `t := out.Task("scan")
t.Doing("walking")
t.Define(scan)
t = out.Task("build")`,
			Remediation:     "Resolve the handle (Define/Fail/Block/Cancel/Skipped) before reassigning the variable, or give the second declaration its own name",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"DOM-019"},
			Since:           "0.2.17",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:        "FP-005",
			Category:  "FP",
			Severity:  SeverityWarning,
			Invariant: "a Task that will complete submits its work through Define — never created already Done",
			Why:       "A tool row that first appears as ✓ looks like a lie: the work happened off-screen. Narrating with Doing before an unrelated Done is the same lie with extra steps (FP-006); the real fix is to let evo run the work via Define.",
			BadCode:   `out.Task("go@1.25.11").Done(path)`,
			GoodCode: `t := out.Task("go@1.25.11")
t.Define(func(ctx context.Context) error {
  return resolve(path)
})`,
			Remediation:     "Call Define(func(ctx context.Context) error { ... }) — with evo.Effect or evo.File inside it for mutations — so evo decides when the row resolves, instead of resolving with Done alone",
			RelatedGuidance: []string{"first-paint", "tasks"},
			VerificationIDs: []string{"FP-005"},
			Since:           "0.4.7",
			Certainty:       CertaintyDeterministic,
		},
		{
			ID:        "FP-006",
			Category:  "FP",
			Severity:  SeverityError,
			Invariant: "Doing narrates work in flight; a Done that immediately follows it with no Define submitting work between them is theater over work that already happened off-row",
			Why:       "`.Doing(\"fixing\").Done(...)` after the fix already ran (zq fix.go:58,265) makes the row narrate a job it never actually gave to evo; FP-005's old suggestion (\"Doing before Done\") prescribed exactly this theater instead of naming Define.",
			BadCode:   `a.out.Task("file integrity").Doing("fixing").Done("%d files changed", fixed)`,
			GoodCode: `t := a.out.Task("file integrity")
t.Define(func(ctx context.Context) error {
  var err error
  fixed, err = quality.Fix(a.services.FS, root, files)
  return err
})`,
			Remediation:     "Replace Doing(...).Done(...) with Define(func(ctx context.Context) error { ... }) so evo — not the caller — decides when the row resolves",
			RelatedGuidance: []string{"first-paint", "tasks"},
			VerificationIDs: []string{"FP-006"},
			Since:           "0.4.7",
			Certainty:       CertaintyDeterministic,
		},
		{
			ID:              "DOM-020",
			Category:        "DOM",
			Severity:        SeverityWarning,
			Invariant:       "usage/user mistakes resolve Block (exit 1); evaluation failures resolve Fail (exit 2)",
			Why:             "Block and Fail carry different exit codes for a reason: a caller reading the exit code needs \"you did something wrong\" (1) and \"something broke while checking\" (2) to stay distinguishable. Routing a usage mistake through Fail reports a user error as a system failure.",
			BadCode:         `task.Fail("missing required --repo flag")`,
			GoodCode:        `task.Block("missing required --repo flag")`,
			Remediation:     "Route usage/user mistakes through Block, not Fail; reserve Fail for evaluation/execution failures",
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
			Invariant: "a Task a function declares is Defined, resolved, or handed on before the function returns",
			Why:       "Wiring a Task's Writer (or adding Facts) does not resolve it: the row stays unresolved, the run concludes partial with 'call Define, Fail, Block, or Skipped on this task', and a child process run off-row never becomes the row's outcome.",
			BadCode: `t := evo.Task("fetch")
cmd.Stdout = t.Writer()
return cmd.Run()`,
			GoodCode: `t := evo.Task("fetch")
t.Define(func(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "git", "fetch")
	cmd.Stdout = t.Writer()
	return cmd.Run()
})`,
			Remediation:     "Run the work inside the Task's Define, or resolve it with Fail, Block, or Skipped",
			RelatedGuidance: []string{"common-api", "tasks"},
			VerificationIDs: []string{"DOM-021"},
			Since:           "1.1.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:        "TAX-003",
			Category:  "TAX",
			Severity:  SeverityWarning,
			Invariant: "a reason used more than as a one-off literal is a compile-time name; a reason names why, not the verb it accompanies",
			Why:       "evo.Reason(\"x\") is legal inline (duplicate strings merge into one bucket), but an inline literal can typo apart into two buckets across call sites, and a reason that only restates the verb (`Skipped(evo.Reason(\"skipped\"))`, zq cmd/zq-build/main.go:81) tells the user nothing they didn't already know from the glyph.",
			BadCode: `task.Skipped(evo.Reason("skipped"))
otherTask.Fact("kept", evo.Reason("kept").Name())`,
			GoodCode: `var reasonProtected = evo.Reason("protected")
task.Skipped(evo.Reason("timeout"))
otherTask.Fact("kept", reasonProtected.Name())`,
			Remediation:     "Lift a repeated reason to a package-level var so it is a compile-time name; name why the item skipped or was kept, not the verb itself",
			RelatedGuidance: []string{"tasks"},
			VerificationIDs: []string{"TAX-003"},
			Since:           "0.4.7",
			Certainty:       CertaintyHeuristic,
		},
	}
}

func init() { registerFamily(domainRules()) }
