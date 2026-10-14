package rules

// apiRules is the API namespace: how callers declare, resolve, and wait on Tasks (API-*, plus the CALL/LOOP/PROG/BOUND call-shape rules).
func apiRules() []Rule {
	return []Rule{
		{
			ID:        "LOOP-001",
			Category:  "LOOP",
			Severity:  SeverityError,
			Invariant: "real work loops live only inside a task definition — never before Task/Group/Sequence",
			Why:       "A for/range that walks disk (or otherwise does the job) before any Task leaves the consumer UI blank: the purge/prune silent-pre-output FAIL class. Group.Task(...).Define per item makes the good path the only path.",
			BadCode: `func previewPurge(ctx context.Context, r *evo.Run) error {
  for _, root := range roots {
    filepath.WalkDir(root, walk) // silent pre-Task loop
  }
  r.Task("inventory").Define(summarize)
  return nil
}`,
			GoodCode: `func previewPurge(ctx context.Context, r *evo.Run) error {
  inv := r.Task("inventory")
  inv.Doing("walking worktrees")
  inv.Define(func(context.Context) error {
    for _, root := range roots {
      if err := filepath.WalkDir(root, walk); err != nil {
        return fmt.Errorf("walk %s: %w", root, err)
      }
    }
    return nil
  })
  return nil
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
			Invariant: "evo.Main/evo.Execute arguments are named values, never inline make/new",
			Why:       "Inline make() or new() inside a Config hides the value the call site is passing, and a buffer or level built there can never be read back after the run. Bind a named local before the call.",
			BadCode:   `res := evo.Execute(ctx, evo.Config{Facades: evo.Facades{Stdout: new(bytes.Buffer)}}, run) // nothing holds the buffer`,
			GoodCode: `stdout := new(bytes.Buffer)
res := evo.Execute(ctx, evo.Config{Facades: evo.Facades{Stdout: stdout}}, run)
fmt.Fprint(os.Stdout, stdout.String())`,
			Remediation:     "Extract make/new to a named local before the evo.Main/evo.Execute call",
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
			Invariant: "process exit codes come only from os.Exit(evo.Main(evo.Config{}, run)), or evo.Execute's returned Result.ExitCode() (evo.MainWith was removed in 1.0)",
			Why:       "A hand-mapped os.Exit int bypasses the Outcome→exit-code contract; a Blocked run (1) can silently read as success, or a real failure can read as blocked.",
			BadCode: `if err != nil {
  fmt.Println(err)
  os.Exit(1) // hand-mapped, not fed by evo
}`,
			GoodCode:        `os.Exit(evo.Main(evo.Config{}, run)) // run(ctx) returns error; Conclusion decides 0/1/2/130; Main derives the code, caller exits`,
			Remediation:     "Route exit through os.Exit(evo.Main(evo.Config{}, run)), or evo.Execute's returned Result.ExitCode() for a caller that needs the code without exiting (evo.MainWith was removed in 1.0)",
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
			Why:       "strings.Join of an unbounded slice dumped into Detail/Doing floods the terminal with every name; Evo folds an Effect's names itself but cannot bound text joined by hand.",
			BadCode: `task.Doing(strings.Join(reasons, "; "))
return evo.Fail("cannot delete", evo.Detail(strings.Join(names, ", ")))`,
			GoodCode: `shown := strings.Join(names[:min(len(names), 8)], ", ")
task.Doing(shown)
return evo.Fail("cannot delete", evo.Detail(shown))`,
			Remediation:     "Bound the joined slice to its first few names plus a count before passing it to evo.Detail or Doing",
			RelatedGuidance: []string{"tasks"},
			VerificationIDs: []string{"BOUND-001"},
			Since:           "0.7.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:        "API-030",
			Category:  "API",
			Severity:  SeverityError,
			Invariant: "every Task is declared in the enclosing function, in order, never inside a goroutine or g.Go closure",
			Why:       "Declaration order is topology: it fixes the row order and which siblings a later After can name. A Task declared inside a worker closure takes its place from goroutine scheduling, so two runs of the same program can show different rows and different edges. Declaring from inside a Define callback is refused outright.",
			BadCode: `work := r.Group("jobs")
for _, j := range jobs {
  go func() {
    work.Task(j.Name).Define(j.Run) // declared inside the goroutine: order follows the scheduler
  }()
}`,
			GoodCode: `work := r.Group("jobs")
for _, j := range jobs {
  work.Task(j.Name).Define(j.Run) // declared in order; Group runs the eligible children concurrently
}`,
			Remediation:     "Declare each Task with Group.Task or Sequence.Task in the enclosing function and give it Define there; delete the goroutine, because Group already overlaps independent Tasks",
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
			Why:       "A caller-defined io.Writer whose Write method calls Task.Doing reimplements the line-splitting adapter Task.Writer already owns.",
			BadCode: `type livePhase struct{ task *evo.Task }
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
			Invariant: "a removed 1.x spelling is migrated to its 2.0 form, never taught: the module path without /v2, the Task methods Block, Blockf, Fail, Failf, Skipped, Kept, Warn, Step, Done, Evidence, Next and NextCommand (removed in 1.1 and 2.0), evo.NextCommand (removed in 2.0), the 1.x entry points evo.Init, evo.New, the package-level and Output Task, Group and Sequence, the one-argument evo.Main(run) over func run(ctx) error, evo.Print* and Config.Title (removed in 2.0), the mutation verbs Add, Create, Delete, Push, Remove, Update and Write with evo.Affected, and Record, RecordLabel and RecordName (removed in 1.1), the Option funcs and Config.Options, and every other export the retired table lists (removed in 1.1, 1.2 or 2.0)",
			Why:       "2.0 is a new contract, so a 1.x call does not compile. Review maps each removed spelling straight to the 2.0 surface, never to a 1.x intermediate: a Task ends by returning evo.Fail, evo.Refuse or evo.Exclude from Define (removed in 2.0: Block, Skipped; removed in 1.1: Failf, Blockf), and work already in place is Task.Verify, which resolves the Task Satisfied without running it (Kept, removed in 1.1); a warning is Task.Problem, which never ends the Task; progress is Progress(completed, total).Doing(item); success is returning from Define with the text as Summary (Done was removed in 1.1); an opaque mutation is evo.Effect(ctx, EffectSpec{Verb, Object, Quantity}, fn) and file state is evo.File (the Task mutation verbs and Record* were removed in 1.1); a remedy is evo.Next(evo.Action{...}) on the ending it explains; retained output is Task.Writer(); and Config and Config.Facades fields replace the Option funcs.",
			BadCode: `task.Block("no remote", evo.NextCommand("git", "remote", "add")) // removed in 2.0
task.Delete("local tip", remove, evo.Affected(n))                // removed in 1.1
task.Warn("stale entry")                                         // removed in 1.1`,
			GoodCode: `func run(ctx context.Context, r *evo.Run) error {
	r.Task("publish").Define(func(ctx context.Context) error {
		return evo.Refuse("no remote", evo.Next(evo.Action{Command: []string{"git", "remote", "add"}}))
	})
	r.Task("branches").Define(func(ctx context.Context) error {
		return evo.Effect(ctx, evo.EffectSpec{Verb: evo.Delete, Object: "local tip", Quantity: n}, remove)
	})
	r.Task("scan").Problem("stale entry")
	return nil
}`,
			Remediation:     "Import the /v2 module path (removed in 2.0: the path without /v2). Replace Block and Blockf with return evo.Refuse(summary, opts...), Fail and Failf with return evo.Fail(summary, opts...), Skipped with return evo.Exclude(reason), and Kept with Task.Verify declared before Define (Block and Skipped removed in 2.0; Blockf, Failf and Kept removed in 1.1). Replace Warn with Task.Problem(summary) and Step with Progress(completed, total).Doing(item) (removed in 1.1). Replace Done() with Define(func(ctx context.Context) error { ... }) and Done(text) with Summary(text) (removed in 1.1). Replace each mutation verb with Define plus evo.Effect(ctx, evo.EffectSpec{Verb, Object, Quantity}, fn), Write with evo.File, and Record, RecordLabel and RecordName with evo.Effect, Task.Fact or evo.File by what they reported (removed in 1.1). Replace evo.Init, evo.New and the one-argument evo.Main(run) (removed in 2.0) with evo.Main(evo.Config{Name: ...}, run) over func run(ctx context.Context, r *evo.Run) error, the package-level and Output Task, Group and Sequence with r.Task, r.Group and r.Sequence, evo.Print* with fmt.Fprint*(r.Text(), ...) and Config.Title with Config.Name (removed in 2.0). A program that imports the 2.0 module and does not compile against it is never clean: review type-checks it against the 2.0 surface and reports each compile error with the replacement the migration guide gives. Replace evo.NextCommand (removed in 2.0) and the Next methods with evo.Next(evo.Action{Command: ...}) on the evo.Fail, evo.Refuse or Task.Problem it explains, Evidence() with Task.Writer(), and Config.Options and the Option funcs with Config and Config.Facades fields.",
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
			Why:       "A Task named note that Excludes with note tells the reader nothing a bare \"skipped 1 (note)\" wouldn't already — the name and the reason/verb argument are the identical expression, so the second one carries zero new information.",
			BadCode:   `r.Task(note).Define(func(context.Context) error { return evo.Exclude(note) })`,
			GoodCode: `r.Task("branch check").Define(func(context.Context) error {
  return evo.Exclude("protected")
})`,
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
			Invariant: "the minimal Task happy path (Main/Task/Define) compiles with a zero Config",
			Why:       "Requiring a populated Config struct for the common single-task check adds ceremony that discourages the minimal, correct spelling.",
			BadCode:   `os.Exit(evo.Main(evo.Config{Name: "tool", Output: evo.Human}, run)) // fields filled in for no reason`,
			GoodCode: `func run(ctx context.Context, r *evo.Run) error {
  r.Task("disk space").Define(checkDiskSpace)
  return nil
}`,
			Remediation:     "Use evo.Main(evo.Config{}) with a zero Config for the minimal happy path; add fields only to override stream or behavior defaults",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"API-001"},
			Since:           "0.1.0",
			Certainty:       CertaintyHeuristic,
			Detection:       DetectionGuidance, // no cheap detector: an empty/default Config literal is not distinguishable from an intentional one by AST alone
		},
		{
			ID:        "API-035",
			Category:  "API",
			Severity:  SeverityWarning,
			Invariant: "a function that Fails/Refuses wires its checked command's output somewhere, never io.Discard",
			Why:       "io.Discard as a sink in a function that also Fails/Refuses is an evidence-free security-gate shape: the verdict has nothing to show for itself when it matters most.",
			BadCode: `cmd.Stdout = io.Discard
if err := cmd.Run(); err != nil {
  return evo.Refuse("policy check failed") // no output kept to show why
}`,
			GoodCode: `cmd.Stdout = task.Writer()
cmd.Stderr = task.Writer()
if err := cmd.Run(); err != nil {
  return evo.Refuse("policy check failed", evo.Detail(err.Error()),
    evo.Next(evo.Action{Label: "run git status"}))
}`,
			Remediation:     "Wire the checked command's output through task.Writer() instead of io.Discard, so Refuse/Fail can attach evidence",
			RelatedGuidance: []string{"streams"},
			VerificationIDs: []string{"API-035"},
			Since:           "0.2.17",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:        "API-037",
			Category:  "API",
			Severity:  SeverityWarning,
			Invariant: "a method that only forwards to one Task method call is inlined at its callers, not wrapped",
			Why:       "A method whose entire body is one call on a Task adds a name and a stack frame with no behavior of its own (zq's resolutionPhase wrapper).",
			BadCode: `func (r *runner) resolutionPhase(text string) {
  r.task.Doing(text)
}`,
			GoodCode:        `app.task.Doing(text) // called directly at each site`,
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
			Why: "Doing already accepts (text string, args ...any) directly — wrapping the call in fmt.Sprintf is ceremony that " +
				"also hides the real arguments from evo's own formatting.",
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
			Why:       "A Group organizes several children. With exactly one it is a lone Task under a second name that competes with the Task's own row for the command name. Declare the Task, or add the other children.",
			BadCode: `jobs := r.Group("run")
jobs.Task("install:fresh-start").Define(install)`,
			GoodCode:        `r.Task("install:fresh-start").Define(install)`,
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
			Invariant: "an ending built with evo.Fail, evo.Refuse or evo.Exclude is returned, never discarded",
			Why:       "evo.Fail, evo.Refuse and evo.Exclude only build the error a Define callback (or the run body) returns; the returned value is what settles the Task's Outcome. Calling one as a statement drops the ending on the floor: the callback then ends with whatever it returns next (nil is Succeeded, a bare err is a Failed Task with no summary), so the summary, Detail, Code and Next the author wrote never reach the row.",
			BadCode: `task.Define(func(ctx context.Context) error {
  if err := resolve(ctx); err != nil {
    evo.Fail("cannot resolve") // result discarded
    return err
  }
  return nil
})`,
			GoodCode: `task.Define(func(ctx context.Context) error {
  if err := resolve(ctx); err != nil {
    return evo.Fail("cannot resolve", evo.Detail(err.Error()),
      evo.Next(evo.Action{Label: "run git status"}))
  }
  return nil
})`,
			Remediation:     "Return the ending: `return evo.Fail(summary, opts...)`, `return evo.Refuse(summary, opts...)` or `return evo.Exclude(reason, opts...)`. To keep an underlying error's text, put it in evo.Detail(err.Error()); never swallow the failure with `return nil`.",
			RelatedGuidance: []string{"tasks", "common-api"},
			VerificationIDs: []string{"API-040"},
			Since:           "0.4.7",
			Certainty:       CertaintyDeterministic,
		},
		{
			ID:        "API-041",
			Category:  "API",
			Severity:  SeverityError,
			Invariant: "a goroutine or g.Go closure never reports on a Task that has no Define; work reaches the scheduler only through Define",
			Why:       "`go func(){ task.Doing(\"x\") }()` over a Task nobody gave Define compiles and looks like work in progress, but the scheduler never received anything to run: the Task stays NotStarted and the run ends partial (zq axis-11 P1).",
			BadCode: `task := r.Task("a")
go func() {
  task.Doing("working")
  task.Progress(1, 2)
}()`,
			GoodCode: `r.Task("a").Define(func(ctx context.Context) error {
  return doWork(ctx)
})`,
			Remediation:     "Declare the work with Group.Task(...) (one named Task per item) and call task.Define(func(ctx context.Context) error { ... }) instead of a bare goroutine. A goroutine a callback starts holds no scheduler slot: if the callback blocks on it while every slot is held, the run stalls. Give each unit of work its own Task and let Evo schedule it",
			RelatedGuidance: []string{"tasks"},
			VerificationIDs: []string{"API-041"},
			Since:           "0.4.7",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:        "API-044",
			Category:  "API",
			Severity:  SeverityError,
			Invariant: "work that must finish before other work is ordered with After and read with Computed.Get; a function never declares work with Define and then blocks on a channel the callback feeds",
			Why:       "Define submits work to the scheduler and returns. A function that then waits on a channel the callback sends to blocks the stack that is still declaring the topology: every Task declared after the wait is unknown to the scheduler and to the dry-run plan until the callback has finished, nothing declared there can overlap it, and under MaxConcurrency 1 a waiter that itself runs inside a callback holds the only slot its own callback needs. After and Computed.Get give the same ordering and hand-off without blocking anything.",
			BadCode: `func defineAndWait(task *evo.Task, fn func(context.Context) error) error {
  done := make(chan error, 1)
  task.Define(func(ctx context.Context) error {
    err := fn(ctx)
    done <- err
    return err
  })
  return <-done
}`,
			GoodCode: `func declare(r *evo.Run, fn func(context.Context) error, next func(context.Context) error) {
  first := r.Task("apply config")
  first.Define(fn)
  r.Task("reload service").After(first).Define(next)
}`,
			Remediation:     "Delete the channel and the receive; keep task.Define(fn), order the work that needed the result with After(task), and pass a produced value with evo.Compute and Computed.Get",
			RelatedGuidance: []string{"tasks"},
			VerificationIDs: []string{"API-044"},
			Since:           "0.4.7",
			Certainty:       CertaintyHeuristic,
		},
	}
}

func init() { registerFamily(apiRules()) }
