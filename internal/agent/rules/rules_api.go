package rules

// apiRules is the API namespace: how callers declare, resolve, and wait on Tasks (API-*, plus the CALL/LOOP/PROG/BOUND call-shape rules).
func apiRules() []Rule {
	return []Rule{
		{
			ID:        "API-006",
			Category:  "API",
			Severity:  SeverityWarning,
			Invariant: "explicit Start is optional",
			Why:       "Doing/Progress/Define already activate the task; Start is redundant noise for agents and readers.",
			BadCode: `t := out.Task("scan")
t.Start()
t.Doing("walking")`,
			GoodCode: `t := out.Task("scan")
t.Doing("walking")`,
			Remediation:     "Use Doing/Progress or Define; remove explicit Start",
			Exceptions:      []string{"tests that assert Start side effects"},
			RelatedGuidance: []string{"tasks", "common-api"},
			VerificationIDs: []string{"MCP-012", "API-006"},
			Since:           "0.1.0",
			Certainty:       CertaintyDeterministic,
		},
		{
			ID:        "API-026",
			Category:  "API",
			Severity:  SeverityError,
			Invariant: "caller-invented RunAll/Map/Retry/Parallel/Timeout are forbidden on evo receivers; Group/Sequence/Define/After are not (Group.Each/Sequence.Each were removed in 1.0 — one named Task per item is the current shape)",
			Why:       "Evo owns scheduling through Group, Sequence, Define, and After. Callers must not invent RunAll/Map/Retry/Parallel/Timeout on evo receivers. Substring detection false-positives on strings.Map; review uses AST on evo receivers only.",
			BadCode: `out.Group("jobs").Map(func() {})
out.Task("x").Retry(3)`,
			GoodCode: `worktrees := evo.Group("worktrees")
for _, path := range paths {
  path := path
  worktrees.Task(path).Define(func(context.Context) error { return check(path) })
}`,
			Remediation:     "Declare one named Task per item under Group/Sequence, then Define/After; do not add RunAll/Map/Retry on evo types",
			RelatedGuidance: []string{"common-api", "tasks"},
			VerificationIDs: []string{"API-026"},
			Since:           "0.1.0",
			Certainty:       CertaintyDeterministic,
		},
		{
			ID:        "API-027",
			Category:  "API",
			Severity:  SeverityError,
			Invariant: "Task cannot contain children; Group/Sequence have no leaf lifecycle",
			Why:       "Collection state is derived from children; calling Done/Fail on Group/Sequence invents false authority.",
			BadCode: `g := out.Group("deps")
g.Done() // forbidden`,
			GoodCode: `g := out.Group("deps")
g.Task("a").Define(installA)
g.Task("b").Define(installB)`,
			Remediation:     "Use Group.Task/Sequence.Task for children; never Done/Fail/Progress on the collection",
			RelatedGuidance: []string{"tasks"},
			VerificationIDs: []string{"API-027", "DOM-016"},
			Since:           "0.1.0",
			Certainty:       CertaintyDeterministic,
		},
		{
			ID:        "API-028",
			Category:  "API",
			Severity:  SeverityWarning,
			Invariant: "Failf/Blockf require a format directive — every other *f method is deleted",
			Why: "Failf(\"boom\") with no directive at all is ceremony; Fail(\"boom\") is the intent. " +
				"C6 deleted Donef/Summaryf/Itemf/Taskf/Tasksf/Changesf/Planf/Warnf/Reasonf entirely — " +
				"Task/Group/Sequence/Changes/Plan/Warn/Reason are printf-variadic themselves now, " +
				"so there is nothing left in that family to flag; Failf/Blockf survive for their %w+*Failure semantics.",
			BadCode: `task.Failf("boom")`,
			GoodCode: `task.Fail("boom")
task.Failf("boom: %w", err)`,
			Remediation:     "Use Fail/Block without f when there is no %w to wrap; Task/Group/Sequence/Changes/Plan/Warn/Reason take printf args directly",
			RelatedGuidance: []string{"tasks", "common-api"},
			VerificationIDs: []string{"API-028"},
			Since:           "0.2.0",
			Certainty:       CertaintyDeterministic,
		},
		{
			ID:        "API-029",
			Category:  "API",
			Severity:  SeverityWarning,
			Invariant: "subprocess evidence uses Task.Writer, not DebugWriter",
			Why:       "DebugWriter is filtered by DebugLevel and is the wrong dialect for failure evidence.",
			BadCode: `dbg := out.DebugWriter()
run.Run(ctx, "brew", args, dbg)`,
			GoodCode: `cmd.Stdout = task.Writer()
cmd.Stderr = task.Writer()
if err := cmd.Run(); err != nil {
	return task.Failf("brew failed: %w", err)
}`,
			Remediation:     "Use cmd.Stdout = task.Writer() + Failf's trailing %w",
			RelatedGuidance: []string{"streams", "tasks"},
			VerificationIDs: []string{"API-029"},
			Since:           "0.2.0",
			Certainty:       CertaintyDeterministic,
		},
		{
			ID:        "LOOP-001",
			Category:  "LOOP",
			Severity:  SeverityError,
			Invariant: "real work loops live only inside a task definition — never before Task/Group/Sequence",
			Why:       "A for/range that walks disk (or otherwise does the job) before any Task leaves the consumer UI blank: the purge/prune silent-pre-output FAIL class. Group.Task(...).Define per item makes the good path the only path.",
			BadCode: `func previewPurge() {
  out := evo.Init(evo.Config{Isolated: true, DryRun: true})
  for _, root := range roots {
    filepath.WalkDir(root, walk) // silent pre-Task loop
  }
  out.Task("inventory").Define(summarize)
}`,
			GoodCode: `func previewPurge() {
  out := evo.Init(evo.Config{Isolated: true, DryRun: true, Facts: rootFacts})
  inv := out.Task("inventory")
  inv.Doing("walking worktrees")
  inv.Define(func(context.Context) error {
    for _, root := range roots {
      if err := filepath.WalkDir(root, walk); err != nil {
        return err
      }
    }
    return nil
  })
}`,
			Remediation:     "Declare Task/Group/Sequence first; put the loop inside Task.Define, or declare one named Task per item under a Group, so every iteration is visible work",
			RelatedGuidance: []string{"first-paint", "tasks"},
			VerificationIDs: []string{"LOOP-001"},
			Since:           "0.5.1",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:        "CALL-001",
			Category:  "CALL",
			Severity:  SeverityWarning,
			Invariant: "evo.Init/Task/Group arguments are named values, never inline make/new",
			Why:       "Inline make() or new() inside an evo construct call hides the value the call site is passing. Extract a named local before the call so the argument list stays readable and reviewable.",
			BadCode: `out := evo.Init(evo.Config{Facts: make([]evo.Fact, 0)})
_ = out.Task("scan")`,
			GoodCode: `facts := make([]evo.FactRecord, 0)
out := evo.Init(evo.Config{Facts: facts})
_ = out.Task("scan")`,
			Remediation:     "Extract make/new to a named local before the evo.Init/Task/Group call",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"CALL-001"},
			Since:           "0.5.1",
			Certainty:       CertaintyDeterministic,
		},
		{
			ID:              "API-000",
			Category:        "API",
			Severity:        SeverityError,
			Invariant:       "source must parse before any other finding is trustworthy",
			Why:             "A parse failure means every AST-based rule below it saw a broken tree; reporting anything else is noise the agent cannot act on.",
			BadCode:         `func f( { // syntax error`,
			GoodCode:        `func f() {} // valid Go`,
			Remediation:     "Fix the reported syntax error and rerun review; no other findings are meaningful until the file parses",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"API-000"},
			Since:           "0.1.0",
			Certainty:       CertaintyDeterministic,
		},
		{
			ID:        "API-018",
			Category:  "API",
			Severity:  SeverityWarning,
			Invariant: "process exit codes come only from os.Exit(evo.Main(run)), or evo.Run/Output.Run's returned Result.ExitCode() (evo.MainWith, which restated the same os.Exit facade for an Isolated *Output, was removed in 1.0 — see Output.Run)",
			Why:       "A hand-mapped os.Exit int bypasses the Outcome→exit-code contract; a Blocked run (1) can silently read as success, or a real failure can read as blocked.",
			BadCode: `if err != nil {
  fmt.Println(err)
  os.Exit(1) // hand-mapped, not fed by evo
}`,
			GoodCode:        `os.Exit(evo.Main(run)) // run(ctx) returns error; Conclusion decides 0/1/2/130; Main derives the code, caller exits`,
			Remediation:     "Route exit through os.Exit(evo.Main(run)), or evo.Run/Output.Run's returned Result.ExitCode() for a caller that needs the code without exiting (evo.MainWith was removed in 1.0)",
			RelatedGuidance: []string{"streams", "common-api"},
			VerificationIDs: []string{"API-018"},
			Since:           "0.2.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:        "PROG-001",
			Category:  "PROG",
			Severity:  SeverityError,
			Invariant: "indeterminate to determinate progress happens once; a sealed total is immutable",
			Why:       "Re-sealing a total (14/40 becoming 14/53) or letting completed exceed total makes the bar impossible to trust.",
			BadCode: `task.Progress(14, 40)
// ...later, denominator recomputed from a fresh scan
task.Progress(14, 53) // second indeterminate->determinate transition: forbidden`,
			GoodCode: `task.Progress(14, 0)  // indeterminate: "14 processed"
task.Progress(14, 40) // sealed once discovery completes; never re-sealed`,
			Remediation:     "Seal the total once discovery completes; never recompute or lower it afterward",
			RelatedGuidance: []string{"tasks"},
			VerificationIDs: []string{"PROG-001"},
			Since:           "0.6.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:        "BOUND-001",
			Category:  "BOUND",
			Severity:  SeverityWarning,
			Invariant: "slice-derived text into Detail/Doing is bounded before rendering",
			Why:       "strings.Join of an unbounded slice dumped into Detail/Doing reproduces the 500-name terminal flood evo-rec.md \"bounded effect rows\" already fixed for Plan/Changes.",
			BadCode: `task.Fail("cannot delete", evo.Detail(strings.Join(names, ", ")))
task.Doing(strings.Join(reasons, "; "))`,
			GoodCode: `task.Fail("cannot delete", evo.Detail(evo.TruncateNames(names, 8)))
task.Doing(evo.TruncateNames(reasons, 8))`,
			Remediation:     "Wrap the joined slice in evo.TruncateNames before passing it to Detail/Doing",
			RelatedGuidance: []string{"tasks"},
			VerificationIDs: []string{"BOUND-001"},
			Since:           "0.7.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:        "API-030",
			Category:  "API",
			Severity:  SeverityError,
			Invariant: "Task/Group.Task is predeclared before fan-out, never called inside the worker closure",
			Why:       "Declaring a Task inside a goroutine or g.Go closure races task creation with rendering and produces the exact unordered five-spinner defect evo-rec.md \"sequential presentation\" forbids.",
			BadCode: `for _, j := range jobs {
  go func(j Job) {
    t := out.Task(j.Name) // declared inside the goroutine: race + unordered
    t.Define(j.Run)
  }(j)
}`,
			GoodCode: `work := out.Group("jobs")
for _, j := range jobs {
  work.Task(j.Name).Define(j.Run) // predeclared, in order; evo's scheduler runs them
}`,
			Remediation:     "Call out.Task/Group.Task for every child before starting any goroutine; pass the handle in",
			RelatedGuidance: []string{"tasks"},
			VerificationIDs: []string{"API-030"},
			Since:           "0.7.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:        "API-031",
			Category:  "API",
			Severity:  SeverityWarning,
			Invariant: "child-process phase narration uses Task.Writer, never a hand-rolled io.Writer",
			Why:       "A caller-defined io.Writer whose Write method calls TaskHandle.Doing reimplements the exact 30-line line-splitting adapter Task.Writer already owns (evo-rec.md \"#6\").",
			BadCode: `type livePhase struct{ task *evo.TaskHandle }
func (w *livePhase) Write(p []byte) (int, error) {
  w.task.Doing(lastLine(p))
  return len(p), nil
}`,
			GoodCode:        `cmd.Stdout = task.Writer() // last child line becomes the live doing-text`,
			Remediation:     "Delete the hand-rolled io.Writer and wire the subprocess's Stdout/Stderr to Task.Writer() directly",
			RelatedGuidance: []string{"tasks", "streams"},
			VerificationIDs: []string{"API-031"},
			Since:           "0.7.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:        "API-032",
			Category:  "API",
			Severity:  SeverityWarning,
			Invariant: "superseded spellings are rewritten, not taught: evo.New, Item/.OK/.Because, Cause, Capture, Config.Options / []evo.Option / Option funcs (To/Plain/NoColor/Stdin/DryRun/VisibilityDelay/Diagnostics), the TaskHandle mutation verbs (Add/Create/Delete/Push/Remove/Update/Write), evo.Affected, and TaskHandle.Done (removed in 1.1), evo.ID, evo.StartPhase, and evo.EntityOption (removed in 1.1), the retired independent-collection constructor, Skip, evo.MainWith (removed in 1.0)",
			Why:       "evo.Init+evo.Main is the sole constructor/ordinary main() lifecycle (New and MainWith were removed in 1.0; Isolated *Output uses Output.Run); Config fields replaced Option funcs; the TaskHandle mutation verbs were removed in 1.1 — an opaque mutation is evo.Effect(ctx, EffectSpec{Verb, Object, Quantity}, fn) inside Define and file state is evo.File, so neither the 0.x positional Delete(n, object) nor the 1.0 Delete(object, fn, Affected(n)) compiles; the independent collection constructor is Group; Item folded into Task; Cause no longer affects the returned error since Fail/Block are statement-form (use Failf/Blockf's trailing %w); Capture was renamed to Evidence — \"Stdout\" would lie as a name since it also takes stderr; Skip is Skipped; ID/StartPhase were removed in 1.1 (Task takes only the name; TaskHandle.Key overrides identity; Doing sets the first phase).",
			BadCode: `func main() {
	out := evo.New(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain()}})
	os.Exit(evo.MainWith(out, run)) // MainWith: removed in 1.0
}
func run(out *evo.Output) error {
	task := out.Task("branches", evo.StartPhase("classifying tips"))
	task.Delete(n, "local tip")
	task.Skip("skipped")
	return out.Task("z").Fail("failed", evo.Cause(err))
}`,
			GoodCode: `func main() {
	evo.Init(evo.Config{Title: "tool", Stdout: &buf, Plain: true})
	os.Exit(evo.Main(run))
}
func run(ctx context.Context) error {
	task := evo.Task("branches")
	task.Doing("classifying tips")
	task.Define(func(ctx context.Context) error {
		return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectDelete, Object: "local tip", Quantity: n}, remove)
	})
	evo.Task("tip").Skipped(reason)
	return evo.Task("z").Failf("failed: %w", err)
}`,
			Remediation:     "Replace evo.New with evo.Init; evo.Main in ordinary main, Output.Run when holding Isolated *Output; replace Config.Options / evo.To/Plain/NoColor with Config fields (Stdout, Plain, Color: ColorNever); replace every TaskHandle mutation verb (removed in 1.1, either shape) with Define + evo.Effect(ctx, evo.EffectSpec{Verb, Object, Quantity}, fn), and Task.Write with evo.File; replace the retired collection constructor with Group; replace Skip with Skipped; drop evo.ID / evo.StartPhase (Doing for the first phase); replace Item(...) with Task(...); replace OK() with Define(func(ctx context.Context) error { ... }); fold Because(text) into Summary(text) or the resolving verb's own argument; replace evo.Cause(err) with Failf/Blockf's trailing \": %w\"; replace .Capture() with task.Writer(); replace TaskHandle.Done() (removed in 1.1) with Define(func(ctx context.Context) error { ... }) and Done(text) with Summary(text) — inside the Task's own Define callback, Summary alone",
			RelatedGuidance: []string{"common-api", "tasks", "streams"},
			VerificationIDs: []string{"API-032"},
			Since:           "0.3.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:        "API-033",
			Category:  "API",
			Severity:  SeverityWarning,
			Invariant: "an entity's name is not also its own skip/verb evidence",
			Why:       "out.Task(note).Skip(note) tells the reader nothing a bare \"skipped 1 (note)\" wouldn't already — the name and the reason/verb argument are the identical expression, so the second one carries zero new information.",
			BadCode:   `out.Task(note).Skip(note)`,
			GoodCode: `item := out.Task("branch check")
item.Skipped(reason)`,
			Remediation:     "Give the entity a real label distinct from the reason/verb text it also carries",
			RelatedGuidance: []string{"tasks", "common-api"},
			VerificationIDs: []string{"API-033"},
			Since:           "0.3.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:        "API-001",
			Category:  "API",
			Severity:  SeverityWarning,
			Invariant: "the minimal Task happy path (Init/Task/Define/Block/Finish) compiles with a zero Config",
			Why:       "Requiring a populated Config struct for the common single-task check adds ceremony that discourages the minimal, correct spelling.",
			BadCode: `out := evo.Init(evo.Config{Title: "tool"}) // fields filled in for no reason
out.Task("disk space").Define(checkDiskSpace)`,
			GoodCode: `out := evo.Init(evo.Config{})
out.Task("disk space").Define(checkDiskSpace)`,
			Remediation:     "Use evo.Init(evo.Config{}) with a zero Config for the minimal happy path; add fields only to override stream or behavior defaults",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"API-001"},
			Since:           "0.1.0",
			Certainty:       CertaintyHeuristic,
			Detection:       DetectionGuidance, // no cheap detector: an empty/default Config literal is not distinguishable from an intentional one by AST alone
		},
		{
			ID:        "API-034",
			Category:  "API",
			Severity:  SeverityError,
			Invariant: "a statement-form Fail/Block that must propagate an error returns that error, not nil",
			Why:       "return nil immediately after Fail/Block discards the error the caller needed to propagate — the most common shape of \"the remedy has nowhere to attach\" (49 dotfiles + 41 zq sites).",
			BadCode: `if err := validate(cfg); err != nil {
  task.Fail("validate policy manifest")
  return nil
}`,
			GoodCode: `if err := validate(cfg); err != nil {
  return task.Failf("validate policy manifest: %w", err)
}`,
			Remediation:     `Replace the Fail/Block + return nil pair with a returned error: inside a Define/mutation callback return fmt.Errorf("<context>: %w", err) and let Define resolve the task (API-040); elsewhere return task.Failf/Blockf("<context>: %w", err)`,
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"API-034"},
			Since:           "0.2.17",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:        "API-035",
			Category:  "API",
			Severity:  SeverityWarning,
			Invariant: "a function that Fails/Blocks wires its checked command's output somewhere, never io.Discard",
			Why:       "io.Discard as a sink in a function that also Fails/Blocks is an evidence-free security-gate shape: the verdict has nothing to show for itself when it matters most.",
			BadCode: `cmd.Stdout = io.Discard
if err := cmd.Run(); err != nil {
  task.Block("policy check failed")
}`,
			GoodCode: `cmd.Stdout = task.Writer()
cmd.Stderr = task.Writer()
if err := cmd.Run(); err != nil {
  return task.Blockf("policy check failed: %w", err).NextCommand("git", "status")
}`,
			Remediation:     "Wire the checked command's output through task.Writer() instead of io.Discard, so Block/Fail can attach evidence",
			RelatedGuidance: []string{"streams"},
			VerificationIDs: []string{"API-035"},
			Since:           "0.2.17",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:              "API-036",
			Category:        "API",
			Severity:        SeverityWarning,
			Invariant:       "a Fail/Block/Warn summary uses the matching *f method instead of hand-calling fmt.Sprintf",
			Why:             "fmt.Sprintf as Fail/Block/Warn's sole argument is ceremony around a formatting method (Failf/Blockf/Warnf) that already exists.",
			BadCode:         `task.Fail(fmt.Sprintf("delete failed on %s", branch))`,
			GoodCode:        `task.Failf("delete failed on %s", branch)`,
			Remediation:     "Replace Fail/Block/Warn(fmt.Sprintf(...)) with the matching Failf/Blockf/Warnf(...) directly",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"API-036"},
			Since:           "0.2.17",
			Certainty:       CertaintyDeterministic,
		},
		{
			ID:        "API-037",
			Category:  "API",
			Severity:  SeverityWarning,
			Invariant: "a method that only forwards to one Task/Item verb call is inlined at its callers, not wrapped",
			Why:       "A method whose entire body is one call on a Task/Item handle adds a name and a stack frame with no behavior of its own (zq's resolutionPhase wrapper).",
			BadCode: `func (r *runner) resolutionPhase(text string) {
  r.task.Doing(text)
}`,
			GoodCode:        `r.task.Doing(text) // called directly at each site`,
			Remediation:     "Inline the wrapped verb call at each caller and delete the wrapper method",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"API-037"},
			Since:           "0.2.17",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:        "API-038",
			Category:  "API",
			Severity:  SeverityWarning,
			Invariant: "fmt.Sprintf(...) is never passed to a method that is already printf-variadic itself",
			Why: "Task/Group/Sequence/Warn/Doing/Failf all already accept " +
				"(format string, args ...any) directly (P1/P2, C6: their separate *f siblings — Warnf included — " +
				"were deleted) — wrapping the call in fmt.Sprintf is ceremony that also hides the real arguments " +
				"from evo's own formatting.",
			BadCode:         `task.Doing(fmt.Sprintf("scanning %s", path))`,
			GoodCode:        `task.Doing("scanning %s", path)`,
			Remediation:     "Flatten fmt.Sprintf(...) into the method's own format + args; never wrap a printf-variadic evo call in fmt.Sprintf",
			RelatedGuidance: []string{"tasks", "common-api"},
			VerificationIDs: []string{"API-038"},
			Since:           "0.4.1",
			Certainty:       CertaintyDeterministic,
		},
		{
			ID:        "API-039",
			Category:  "API",
			Severity:  SeverityWarning,
			Invariant: "a Group with exactly one child is a lone Task",
			Why:       "A 1-child group paints a 0/1 complete header over a single row and steals the command name. Use Task, or add more children.",
			BadCode: `jobs := out.Group("run")
jobs.Task("install:fresh-start")`,
			GoodCode: `t := out.Task("install:fresh-start")
t.Doing("running install:fresh-start")`,
			Remediation:     "Replace the 1-child Group with a lone Task",
			RelatedGuidance: []string{"tasks"},
			VerificationIDs: []string{"API-039"},
			Since:           "0.4.7",
			Certainty:       CertaintyDeterministic,
		},
		{
			ID:        "API-040",
			Category:  "API",
			Severity:  SeverityError,
			Invariant: "Failf/Blockf inside a Define or mutation callback whose return value reaches that same callback resolves the task twice",
			Why:       "Define's own contract is \"a non-nil return fails the task\"; calling Failf/Fail on the same task and then also returning that error double-resolves it — the row is correct but a spurious second misuse line appears, and zq's taskAlreadyResolved guard exists only to paper over this (app.go:162-167).",
			BadCode: `task.Define(func(ctx context.Context) error {
  if err := a.executeCommand(ctx, root, task, item); err != nil {
    return task.Failf("resolve %s: %w", item.Name, err)
  }
  return nil
})`,
			GoodCode: `task.Define(func(ctx context.Context) error {
  if err := a.executeCommand(ctx, root, task, item); err != nil {
    return err // Define's own non-nil-return-fails-the-task resolves it once
  }
  return nil
})`,
			Remediation:     "Inside a Define/mutation callback, return the error and let Define resolve the task; do not call Failf/Fail on the same task first",
			RelatedGuidance: []string{"tasks", "common-api"},
			VerificationIDs: []string{"API-040"},
			Since:           "0.4.7",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:        "API-041",
			Category:  "API",
			Severity:  SeverityError,
			Invariant: "a goroutine/fan-out closure resolves a predeclared Task (Doing/Fail/Progress) only through Define; a bare go func/.Go(func with no Define races the scheduler",
			Why:       "`go func(){ task.Doing(\"x\"); task.Fail(\"x\") }()` over a predeclared Task compiles and renders identically to scheduled work (zq axis-11 P1) — nothing tells the author evo never scheduled it, so the row and the actual concurrency model silently disagree.",
			BadCode: `t := out.Task("a")
go func() {
  t.Doing("working")
  t.Fail("work failed")
}()`,
			GoodCode: `work := out.Group("work")
for _, name := range []string{"a"} {
  work.Task(name).Define(func(ctx context.Context) error { return doWork(ctx, name) })
}`,
			Remediation:     "Predeclare with Group.Task(...) (one named Task per item), then call task.Define(func() error { ... }) instead of a bare goroutine",
			RelatedGuidance: []string{"tasks"},
			VerificationIDs: []string{"API-041"},
			Since:           "0.4.7",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:        "API-044",
			Category:  "API",
			Severity:  SeverityError,
			Invariant: "a caller waiting for a Define result on this stack uses task.Wait(); a hand-rolled channel wrapper around Define hangs when the task is already terminal",
			Why:       "zq's defineAndWait (setup_python.go:190-210) — make(chan error, 1) + Define + <-done — hangs when the task is already terminal before Define runs (submitWork never calls fn) and deadlocks when nested under MaxConcurrency:1 (axis-3, axis-15, P15/P16 confirmed).",
			BadCode: `done := make(chan error, 1)
task.Define(func() error {
  err := fn()
  done <- err
  return err
})
return <-done`,
			GoodCode: `task.Define(fn)
return task.Wait()`,
			Remediation:     "Replace the make(chan error)/Define/<-done wrapper with task.Define(fn); task.Wait()",
			RelatedGuidance: []string{"tasks"},
			VerificationIDs: []string{"API-044"},
			Since:           "0.4.7",
			Certainty:       CertaintyHeuristic,
			// Wait is being added to the public API in parallel with this
			// rule; this entry documents the spelling the MCP now teaches.
		},
	}
}

func init() { registerFamily(apiRules()) }
