// Package rules is the stable review-rule registry (Appendix C namespaces).
package rules

// Rule is one stable diagnostic rule (MCP-027 / §28.4.5 / Appendix C).
type Rule struct {
	ID              string   `json:"id"`
	Category        string   `json:"category"`
	Severity        string   `json:"severity"`
	Invariant       string   `json:"invariant"`
	Why             string   `json:"why"`
	BadCode         string   `json:"bad_code"`
	GoodCode        string   `json:"good_code"`
	BadOutput       string   `json:"bad_output,omitempty"`
	GoodOutput      string   `json:"good_output,omitempty"`
	Remediation     string   `json:"remediation"`
	Exceptions      []string `json:"exceptions,omitempty"`
	RelatedGuidance []string `json:"related_guidance,omitempty"`
	VerificationIDs []string `json:"verification_ids,omitempty"`
	Since           string   `json:"since"` // first version (MCP-028)
	Deprecated      bool     `json:"deprecated"`
	Replacement     string   `json:"replacement,omitempty"`
	Certainty       string   `json:"certainty,omitempty"` // deterministic | heuristic
	// Detection is "guidance" when no cheap, honest static detector exists
	// for this rule — agent/review never emits this ID, and the catalog/
	// docs teach it by example only. Empty means a detector may exist;
	// callers that need a hard guarantee check agent/review's registered
	// rule IDs directly.
	Detection string `json:"detection,omitempty"`
}

// All returns the v1 rule registry subset. IDs and meanings obey version policy:
// IDs never rename; deprecations dual-write via Deprecated+Replacement (MCP-028).
func All() []Rule {
	return []Rule{
		{
			ID:        "API-006",
			Category:  "API",
			Severity:  "warning",
			Invariant: "explicit Start is optional",
			Why:       "Phase/Progress/Done already activate the task; Start is redundant noise for agents and readers.",
			BadCode: `t := out.Task("scan")
t.Start()
t.Phase("walking")`,
			GoodCode: `t := out.Task("scan")
t.Phase("walking")`,
			Remediation:     "Use Phase/Progress or direct Done/OK; remove explicit Start",
			Exceptions:      []string{"tests that assert Start side effects"},
			RelatedGuidance: []string{"tasks", "common-api"},
			VerificationIDs: []string{"MCP-012", "API-006"},
			Since:           "0.1.0",
			Certainty:       "deterministic",
		},
		{
			ID:        "API-026",
			Category:  "API",
			Severity:  "error",
			Invariant: "evo has no execution helpers (RunAll/Map/Retry/Parallel/Timeout)",
			Why:       "Presentation library must not grow schedulers. Substring detection false-positives on strings.Map; review uses AST on evo receivers only.",
			BadCode: `out.Tasks("jobs").Map(func() {})
out.Task("x").Retry(3)`,
			GoodCode: `// application owns execution
for _, j := range jobs {
  t := out.Task(j.Name)
  if err := j.Run(); err != nil {
    t.Fail(err.Error())
    continue
  }
  t.Done()
}`,
			Remediation:     "Keep loops/retries/timeouts in application code; resolve Task/Item outcomes only",
			RelatedGuidance: []string{"common-api", "tasks"},
			VerificationIDs: []string{"API-026"},
			Since:           "0.1.0",
			Certainty:       "deterministic",
		},
		{
			ID:        "API-027",
			Category:  "API",
			Severity:  "error",
			Invariant: "Task cannot contain children; Tasks has no leaf lifecycle",
			Why:       "Collection state is derived from children; calling Done/Fail on Tasks invents false authority.",
			BadCode: `g := out.Tasks("deps")
g.Done() // forbidden`,
			GoodCode: `g := out.Tasks("deps")
g.Task("a").Done()
g.Task("b").Done()`,
			Remediation:     "Use Tasks.Task for children; never Done/Fail/Progress on the collection",
			RelatedGuidance: []string{"tasks"},
			VerificationIDs: []string{"API-027", "DOM-016"},
			Since:           "0.1.0",
			Certainty:       "deterministic",
		},
		{
			ID:        "STREAM-003",
			Category:  "STREAM",
			Severity:  "error",
			Invariant: "progress must not contaminate structured stdout",
			Why:       "fmt.Print during live UI corrupts managed streams and breaks machine consumers.",
			BadCode: `out := evo.New()
fmt.Printf("progress %d\n", n)`,
			GoodCode: `out := evo.New()
out.Printf("progress %d\n", n)
// or out.Verbose().Println(...) for optional domain detail
// or slog via out.SlogHandler for implementation diagnostics`,
			BadOutput:       "interleaved ANSI + printf on stdout",
			GoodOutput:      "managed Print / Verbose / slog only",
			Remediation:     "Use out.Print/Printf/Println (or Verbose) for human text; slog for diagnostics; Task.Capture for subprocesses",
			RelatedGuidance: []string{"streams", "common-api"},
			VerificationIDs: []string{"STREAM-003", "MCP-013"},
			Since:           "0.1.0",
			Certainty:       "deterministic",
		},
		{
			ID:        "API-028",
			Category:  "API",
			Severity:  "warning",
			Invariant: "formatting *f methods require a format directive",
			Why:       "Donef(\"ok\") is ceremony; Done(\"ok\") is the intent. *f without % confuses readers and agents.",
			BadCode:   `task.Donef("modules cached")`,
			GoodCode: `task.Done("modules cached")
task.Donef("%d packages", n)`,
			Remediation:     "Use Done/Line/Item without f when there is no format verb",
			RelatedGuidance: []string{"tasks", "common-api"},
			VerificationIDs: []string{"API-028"},
			Since:           "0.2.0",
			Certainty:       "deterministic",
		},
		{
			ID:        "API-029",
			Category:  "API",
			Severity:  "warning",
			Invariant: "subprocess evidence uses Task.Evidence, not DebugWriter",
			Why:       "DebugWriter is filtered by DebugLevel and is the wrong dialect for failure Detail tails.",
			BadCode: `dbg := out.DebugWriter()
run.Run(ctx, "brew", args, dbg)`,
			GoodCode: `proof := task.Evidence()
if err := run.Run(ctx, "brew", args, proof); err != nil {
	return task.Failf("brew failed: %w", err) // wrapped error renders as an evidence line
}`,
			Remediation:     "Use task.Evidence() (or task.Run for an *exec.Cmd) + Failf's trailing %w",
			RelatedGuidance: []string{"streams", "tasks"},
			VerificationIDs: []string{"API-029"},
			Since:           "0.2.0",
			Certainty:       "deterministic",
		},
		{
			ID:              "SEC-001",
			Category:        "SEC",
			Severity:        "error",
			Invariant:       "untrusted text cannot control the terminal",
			Why:             "Raw ESC/CSI from user data can hijack the terminal or inject fake UI.",
			BadCode:         `out.Item(userInput).OK() // userInput may contain ESC`,
			GoodCode:        `// library sanitizes names; never write raw ESC to the terminal yourself`,
			Remediation:     "Sanitize caller text; use Detail/Cause split",
			RelatedGuidance: []string{"security"},
			VerificationIDs: []string{"SEC-001", "TXT-007"},
			Since:           "0.1.0",
			Certainty:       "deterministic",
		},
		{
			ID:        "DOM-011",
			Category:  "DOM",
			Severity:  "error",
			Invariant: "expected blocked items are presentation outcomes, not Go application errors",
			Why:       "Block means evaluation succeeded and found a blocker. Returning errors.New after Block confuses agents and callers about failure vs blocked.",
			BadCode: `it := out.Item("working tree")
it.Block("dirty")
return errors.New("dirty") // wrong: blocked is not an app error`,
			GoodCode: `it := out.Item("working tree")
it.Block("dirty")
if err := out.Finish(); err != nil {
  return err // misuse only
}
os.Exit(out.Conclusion().ExitCode) // or return nil to caller that checks ExitCode`,
			Remediation:     "After Block, Finish and use Conclusion.ExitCode; return nil for successful evaluation that found a blocker",
			Exceptions:      []string{"wrapping Finish misuse errors", "I/O failures unrelated to Block"},
			RelatedGuidance: []string{"common-api", "items"},
			VerificationIDs: []string{"MCP-014", "DOM-011", "DOM-048"},
			Since:           "0.1.0",
			Certainty:       "heuristic",
		},
		{
			ID:              "TERM-001",
			Category:        "TERM",
			Severity:        "warning",
			Invariant:       "instant completion does not flash spinner",
			Why:             "Sub-threshold Done should not paint a live spinner that disappears immediately.",
			BadCode:         `// custom spinner without VisibilityDelay`,
			GoodCode:        `// rely on evo VisibilityDelay (default 80ms)`,
			Remediation:     "Rely on visibility delay; do not paint custom spinners",
			RelatedGuidance: []string{"interactive"},
			VerificationIDs: []string{"TERM-001", "H.17"},
			Since:           "0.1.0",
			Certainty:       "deterministic",
		},
		{
			ID:        "FP-001",
			Category:  "FP",
			Severity:  "warning",
			Invariant: "visible state paints within 100ms of process start",
			Why:       "A blank terminal for the first seconds of a run is indistinguishable from a hang; the user re-runs or ^C's healthy work.",
			BadCode: `func main() {
  cfg := loadConfig() // seconds of I/O, nothing on screen yet
  out := evo.New(evo.Config{Title: "tool"})
  out.Task("scan")
}`,
			GoodCode: `func main() {
  evo.Init(evo.Config{Title: "tool"}) // arms first paint before any I/O
  evo.Task("scan").Phase("reading config")
  cfg := loadConfig()
}`,
			Remediation:     "Call evo.Init as the first statement in main and declare the first Task/Item/Group before any I/O",
			RelatedGuidance: []string{"first-paint", "common-api"},
			VerificationIDs: []string{"FP-001"},
			Since:           "0.3.0",
			Certainty:       "heuristic",
		},
		{
			ID:        "FP-002",
			Category:  "FP",
			Severity:  "warning",
			Invariant: "no I/O before the first entity is declared",
			Why:       "Declare-before-compute is what makes FP-001 achievable; I/O ahead of the first Task/Item/Group reintroduces the blank window.",
			BadCode: `func main() {
  evo.Init(evo.Config{Title: "tool"})
  data, _ := os.ReadFile("config.toml") // I/O before any Task/Item/Group
  evo.Task("scan")
}`,
			GoodCode: `func main() {
  evo.Init(evo.Config{Title: "tool"})
  scan := evo.Task("scan")
  scan.Phase("reading config")
  data, _ := os.ReadFile("config.toml")
}`,
			Remediation:     "Move the first Task/Item/Group declaration ahead of the first read/open/dial in main or run",
			RelatedGuidance: []string{"first-paint"},
			VerificationIDs: []string{"FP-002"},
			Since:           "0.3.0",
			Certainty:       "heuristic",
		},
		{
			ID:        "FP-003",
			Category:  "FP",
			Severity:  "warning",
			Invariant: "phases advance on evidence; a stale phase is a defect",
			Why:       "A spinner whose text never changes is animation, not evidence — the user cannot tell slow from hung.",
			BadCode: `t := evo.Task("salvage")
t.Phase("uploading")
run.Run(ctx, "git", args, io.Discard) // silent for minutes; Phase never refreshed`,
			GoodCode: `t := evo.Task("salvage")
t.Phase("uploading")
run.Run(ctx, "git", args, t.PhaseWriter()) // last child line becomes the live Phase
// or stream discovery into Progress as totals become known`,
			Remediation:     "Wire child output through Task.PhaseWriter, or advance Phase/Progress as evidence arrives; the built-in heartbeat covers the remaining silent case (>~10s) automatically",
			RelatedGuidance: []string{"first-paint", "tasks"},
			VerificationIDs: []string{"FP-003"},
			Since:           "0.3.0",
			Certainty:       "heuristic",
		},
		{
			ID:              "API-000",
			Category:        "API",
			Severity:        "error",
			Invariant:       "source must parse before any other finding is trustworthy",
			Why:             "A parse failure means every AST-based rule below it saw a broken tree; reporting anything else is noise the agent cannot act on.",
			BadCode:         `func f( { // syntax error`,
			GoodCode:        `func f() { // valid Go`,
			Remediation:     "Fix the reported syntax error and rerun review; no other findings are meaningful until the file parses",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"API-000"},
			Since:           "0.1.0",
			Certainty:       "deterministic",
		},
		{
			ID:        "API-018",
			Category:  "API",
			Severity:  "warning",
			Invariant: "process exit codes come only from evo.Main/MainWith or Conclusion.ExitCode",
			Why:       "A hand-mapped os.Exit int bypasses the Outcome→exit-code contract; a Blocked run (1) can silently read as success, or a real failure can read as blocked.",
			BadCode: `if err != nil {
  fmt.Println(err)
  os.Exit(1) // hand-mapped, not fed by evo
}`,
			GoodCode:        `os.Exit(evo.Main(run)) // run returns error; Conclusion decides 0/1/2/130`,
			Remediation:     "Route exit through evo.Main/evo.MainWith, or read Conclusion().ExitCode",
			RelatedGuidance: []string{"streams", "common-api"},
			VerificationIDs: []string{"API-018"},
			Since:           "0.2.0",
			Certainty:       "heuristic",
		},
		{
			ID:              "DOM-014",
			Category:        "DOM",
			Severity:        "error",
			Invariant:       "Detail is user-visible string; use Cause(err) for diagnostic errors",
			Why:             "Detail(err) exposes error internals as UI copy and skips the Cause path that diagnostic capture relies on.",
			BadCode:         `it.Block("dirty", evo.Detail(err))`,
			GoodCode:        `it.Block("dirty", evo.Cause(err))`,
			Remediation:     "Replace Detail(err) with Cause(err); reserve Detail for user-visible strings",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"DOM-014"},
			Since:           "0.1.0",
			Certainty:       "heuristic",
		},
		{
			ID:              "MCP-017",
			Category:        "MCP",
			Severity:        "warning",
			Invariant:       "cross-file package review resolves shared types across files when possible",
			Why:             "Without cross-file resolution, collection-leaf misuse (API-027) and other typed checks can miss real defects hidden behind local type aliases.",
			BadCode:         `// caller submits files independently to GoSource, losing cross-file type info`,
			GoodCode:        `// caller submits the whole package via GoPackage(files) so types resolve across files`,
			Remediation:     "Use GoPackage for multi-file review; treat MCP-017 as a partial-coverage signal, not a defect",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"MCP-017"},
			Since:           "0.4.0",
			Certainty:       "deterministic",
		},
		{
			ID:        "SIG-001",
			Category:  "SIG",
			Severity:  "warning",
			Invariant: "signal handling reconciles through Cancel, not a bespoke exit path",
			Why:       "evo.Main/MainWith already wires SIGINT/SIGTERM into Cancel so the ledger's ■ glyph and the 130 exit code agree; a hand-rolled signal.Notify without Cancel reopens that gap.",
			BadCode: `c := make(chan os.Signal, 1)
signal.Notify(c, syscall.SIGINT)
go func() { <-c; os.Exit(1) }()`,
			GoodCode: `c := make(chan os.Signal, 1)
signal.Notify(c, syscall.SIGINT)
go func() { <-c; task.Cancel("interrupted") }()
// or prefer evo.Main/evo.MainWith, which wires this automatically`,
			Remediation:     "Call Cancel on the active task/handle from the signal goroutine, or use evo.Main/MainWith",
			RelatedGuidance: []string{"streams", "interactive"},
			VerificationIDs: []string{"SIG-001"},
			Since:           "0.4.0",
			Certainty:       "heuristic",
		},
		{
			ID:              "TERM-008",
			Category:        "TERM",
			Severity:        "error",
			Invariant:       "cursor hide is always paired with cursor show in a transcript",
			Why:             "An unmatched hide sequence leaves the terminal cursor invisible after the process exits, corrupting the user's shell.",
			BadCode:         `// transcript: \x1b[?25l ... (no matching \x1b[?25h)`,
			GoodCode:        `// transcript: \x1b[?25l ... \x1b[?25h paired on every live region open/close`,
			Remediation:     "Ensure every live-region start restores the cursor on exit, including error/interrupt paths",
			RelatedGuidance: []string{"interactive"},
			VerificationIDs: []string{"TERM-008"},
			Since:           "0.1.0",
			Certainty:       "deterministic",
		},
		{
			ID:              "TERM-014",
			Category:        "TERM",
			Severity:        "warning",
			Invariant:       "transcripts contain only text evo itself wrote through managed streams",
			Why:             "A NUL byte means something wrote raw/binary data into the terminal stream outside evo's sanitize path.",
			BadCode:         `// transcript contains \x00 from an unmanaged binary write`,
			GoodCode:        `// all writes go through out.Print*/task.Capture/PhaseWriter, which sanitize text before it reaches the terminal`,
			Remediation:     "Route the binary-producing writer through Capture/PhaseWriter or a text-only channel; never write raw child bytes straight to the terminal",
			RelatedGuidance: []string{"interactive", "streams"},
			VerificationIDs: []string{"TERM-014"},
			Since:           "0.1.0",
			Certainty:       "heuristic",
		},
		{
			ID:              "SCHEMA-001",
			Category:        "SCHEMA",
			Severity:        "error",
			Invariant:       "structured snapshot documents declare schema_version and a conclusion object",
			Why:             "Machine consumers need a stable version marker and a conclusion payload to parse snapshots safely across releases.",
			BadCode:         `{"foo": 1}`,
			GoodCode:        `{"schema_version": "1", "conclusion": {"outcome": "ok", "exit_code": 0}}`,
			Remediation:     "Emit schema_version and a conclusion object in every structured snapshot/document",
			RelatedGuidance: []string{"streams"},
			VerificationIDs: []string{"SCHEMA-001"},
			Since:           "0.1.0",
			Certainty:       "deterministic",
		},
		{
			ID:        "TERM-015",
			Category:  "TERM",
			Severity:  "warning",
			Invariant: "tty-passthrough child processes run inside out.Suspend",
			Why:       "A child that paints its own UI on the shared terminal collides with the parent's live spinner; no in-process fix helps once the child owns stdout/stderr directly (evo-rec.md \"#7b\").",
			BadCode: `cmd := exec.Command("zq", "setup")
cmd.Stdout = os.Stdout
cmd.Stderr = os.Stderr
cmd.Run()`,
			GoodCode: `cmd := exec.Command("zq", "setup")
cmd.Stdout = os.Stdout
cmd.Stderr = os.Stderr
out.Suspend(func() error { return cmd.Run() })`,
			Remediation:     "Wrap tty-passthrough child execution in out.Suspend(fn); captured children (PhaseWriter/Capture) don't need it",
			RelatedGuidance: []string{"interactive"},
			VerificationIDs: []string{"TERM-015"},
			Since:           "0.6.0",
			Certainty:       "heuristic",
		},
		{
			ID:        "CONFIRM-001",
			Category:  "CONFIRM",
			Severity:  "warning",
			Invariant: "confirmation gates go through evo.Confirm, not a hand-rolled stdin prompt",
			Why:       "A hand-rolled bufio/fmt.Scan prompt redraws under the live spinner, hangs CI when non-interactive, and reports a declined answer as a Go error instead of Blocked.",
			BadCode: `reader := bufio.NewReader(os.Stdin)
fmt.Print("delete origin/production-hotfix? [y/N] ")
answer, _ := reader.ReadString('\n')`,
			GoodCode:        `ok := evo.Confirm("delete origin/production-hotfix?", evo.AssumeYes(flagYes))`,
			Remediation:     "Replace hand-rolled stdin prompts with evo.Confirm; it owns spinner pause, the prompt line, and OK/⊘ resolution",
			RelatedGuidance: []string{"interactive", "common-api"},
			VerificationIDs: []string{"CONFIRM-001"},
			Since:           "0.6.0",
			Certainty:       "heuristic",
		},
		{
			ID:              "GLYPH-001",
			Category:        "GLYPH",
			Severity:        "warning",
			Invariant:       "glyph selection uses a terminal capability profile, measured in cells, not rune counts",
			Why:             "■ ○ → … are East Asian Ambiguous-width families; counting runes instead of measuring terminal cells misjudges layout on affected terminals.",
			BadCode:         `if len([]rune(sym)) == 1 { return sym } // rune count guesses width`,
			GoodCode:        `glyph := profile.Select(evo.StateCancelled) // capability profile measures terminal cells, chooses Neutral/Narrow-width symbols`,
			Remediation:     "Select glyphs via the capability profile (glyphs=auto|unicode|ascii); never infer width from rune count",
			RelatedGuidance: []string{"interactive"},
			VerificationIDs: []string{"GLYPH-001"},
			Detection:       "guidance", // no cheap single-file detector: cell-width vs rune-count is a runtime measurement question, not a static AST pattern
			Since:           "0.6.0",
			Certainty:       "heuristic",
		},
		{
			ID:        "TAX-001",
			Category:  "TAX",
			Severity:  "warning",
			Invariant: "reason partitions sum to the headline count; taxonomy is derived, never hand-assembled",
			Why:       "A bare \"skipped 6\" or a hand-built \"already mutated\" string can't be trusted — it can miscount, and the user can't tell why items were skipped.",
			BadCode:   `msg := fmt.Sprintf("skipped %d", n) // hand-assembled, no reason partition`,
			GoodCode: `task.Skipped(evo.Reason("protected"), "main")
task.Skipped(evo.Reason("dirty"), "feature/x")
// evo derives "skipped 2 (1 protected, 1 dirty)" and enforces 1+1==2`,
			Remediation:     "Record reason + name via task.Skipped/Kept; let evo count, sum, and print the partition",
			RelatedGuidance: []string{"tasks"},
			VerificationIDs: []string{"TAX-001"},
			Since:           "0.6.0",
			Certainty:       "heuristic",
		},
		{
			ID:        "PROG-001",
			Category:  "PROG",
			Severity:  "error",
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
			Certainty:       "heuristic",
		},
		{
			ID:        "CON-001",
			Category:  "CON",
			Severity:  "error",
			Invariant: "Partial is a completeness modifier; exit codes come from Outcome alone, and 130 is reserved for interruption",
			Detection: "guidance", // no cheap single-file detector: correct exit code use is unobservable from source (evo.Main hides the mapping)
			Why:       "Hand-mapping an exit code outside 0/1/2/130, or using 130 for something other than an actual interrupt, breaks the contract wrapping scripts and CI rely on.",
			BadCode: `if blocked {
  os.Exit(3) // hand-mapped code outside Outcome's 0/1/2/130
}
if partial {
  os.Exit(130) // Partial is not interruption
}`,
			GoodCode:        `os.Exit(evo.Main(run)) // exit code derives from Outcome alone: 0 OK / 1 Blocked / 2 Failed / 130 Cancelled; Partial is a completeness modifier only`,
			Remediation:     "Let evo.Main/MainWith pick the exit code from Outcome; never hand-map an int, and never use 130 outside real interruption",
			RelatedGuidance: []string{"streams"},
			VerificationIDs: []string{"CON-001"},
			Since:           "0.6.0",
			Certainty:       "heuristic",
		},
		{
			ID:        "BOUND-001",
			Category:  "BOUND",
			Severity:  "warning",
			Invariant: "slice-derived text into Because/Detail/Phase is bounded before rendering",
			Why:       "strings.Join of an unbounded slice dumped into Because/Detail/Phase reproduces the 500-name terminal flood evo-rec.md \"bounded effect rows\" already fixed for Plan/Changes.",
			BadCode: `task.Fail("cannot delete", evo.Detail(strings.Join(names, ", ")))
item.Because(strings.Join(reasons, "; "))`,
			GoodCode: `task.Fail("cannot delete", evo.Detail(evo.TruncateNames(names, 8)))
item.Because(evo.TruncateNames(reasons, 8))`,
			Remediation:     "Wrap the joined slice in evo.TruncateNames before passing it to Because/Detail/Phase",
			RelatedGuidance: []string{"tasks", "items"},
			VerificationIDs: []string{"BOUND-001"},
			Since:           "0.7.0",
			Certainty:       "heuristic",
		},
		{
			ID:        "API-030",
			Category:  "API",
			Severity:  "error",
			Invariant: "Task/Tasks.Task is predeclared before fan-out, never called inside the worker closure",
			Why:       "Declaring a Task inside a goroutine or g.Go closure races task creation with rendering and produces the exact unordered five-spinner defect evo-rec.md \"sequential presentation\" forbids.",
			BadCode: `for _, j := range jobs {
  go func(j Job) {
    t := out.Task(j.Name) // declared inside the goroutine: race + unordered
    t.Done()
  }(j)
}`,
			GoodCode: `tasks := make([]*evo.TaskHandle, len(jobs))
for i, j := range jobs {
  tasks[i] = out.Task(j.Name) // predeclared, in order, before fan-out
}
for i, j := range jobs {
  go func(i int, j Job) { tasks[i].Done() }(i, j)
}`,
			Remediation:     "Call out.Task/Tasks.Task for every child before starting any goroutine; pass the handle in",
			RelatedGuidance: []string{"tasks"},
			VerificationIDs: []string{"API-030"},
			Since:           "0.7.0",
			Certainty:       "heuristic",
		},
		{
			ID:        "API-031",
			Category:  "API",
			Severity:  "warning",
			Invariant: "child-process phase narration uses Task.PhaseWriter, never a hand-rolled io.Writer",
			Why:       "A caller-defined io.Writer whose Write method calls TaskHandle.Phase reimplements the exact 30-line line-splitting adapter Task.PhaseWriter already owns (evo-rec.md \"#6\").",
			BadCode: `type livePhase struct{ task *evo.TaskHandle }
func (w *livePhase) Write(p []byte) (int, error) {
  w.task.Phase(lastLine(p))
  return len(p), nil
}`,
			GoodCode:        `cmd.Stdout = task.PhaseWriter() // last child line becomes the live Phase`,
			Remediation:     "Delete the hand-rolled io.Writer and wire the subprocess's Stdout/Stderr to Task.PhaseWriter() directly",
			RelatedGuidance: []string{"tasks", "streams"},
			VerificationIDs: []string{"API-031"},
			Since:           "0.7.0",
			Certainty:       "heuristic",
		},
		{
			ID:        "API-032",
			Category:  "API",
			Severity:  "warning",
			Invariant: "evo.New-in-main, MainWith, Cause, and Capture are superseded spellings",
			Why:       "evo.Init+evo.Main is the ordinary main() lifecycle (New/MainWith is the advanced hosted-instance form); Cause no longer affects the returned error since Fail/Block are statement-form (use Failf/Blockf's trailing %w); Capture was renamed to Evidence — \"Stdout\" would lie as a name since it also takes stderr.",
			BadCode: `func main() {
	out := evo.New(evo.Config{Title: "tool"})
	os.Exit(evo.MainWith(out, run))
}
func run(out *evo.Output) error {
	output := out.Task("x").Capture()
	return out.Task("x").Fail("failed", evo.Cause(err))
}`,
			GoodCode: `func main() {
	evo.Init(evo.Config{Title: "tool"})
	os.Exit(evo.Main(run))
}
func run() error {
	proof := evo.Task("x").Evidence()
	return evo.Task("x").Failf("failed: %w", err)
}`,
			Remediation:     "Replace New+MainWith in main with Init+Main; replace evo.Cause(err) with Failf/Blockf's trailing \": %w\"; replace .Capture() with .Evidence()",
			RelatedGuidance: []string{"common-api", "tasks", "streams"},
			VerificationIDs: []string{"API-032"},
			Since:           "0.3.0",
			Certainty:       "heuristic",
		},
		{
			ID:        "API-033",
			Category:  "API",
			Severity:  "warning",
			Invariant: "an entity's name is not also its own skip/verb evidence",
			Why:       "out.Item(note).Skip(note) tells the reader nothing a bare \"skipped 1 (note)\" wouldn't already — the name and the reason/verb argument are the identical expression, so the second one carries zero new information.",
			BadCode:   `out.Item(note).Skip(note)`,
			GoodCode: `item := out.Item("branch check")
item.Skip(note)`,
			Remediation:     "Give the entity a real label distinct from the reason/verb text it also carries",
			RelatedGuidance: []string{"tasks", "common-api"},
			VerificationIDs: []string{"API-033"},
			Since:           "0.3.0",
			Certainty:       "heuristic",
		},
		{
			ID:              "CONFIRM-002",
			Category:        "CONFIRM",
			Severity:        "warning",
			Invariant:       "a destructive confirm question is marked evo.Destructive()",
			Why:             "A remote force-delete or trash confirm rendered like an ordinary yes/no question lets a user approve a severe action without the \"(destructive)\" cue evo-rec.md \"confirm gate\" requires.",
			BadCode:         `evo.Confirm("delete origin/production-hotfix?")`,
			GoodCode:        `evo.Confirm("delete origin/production-hotfix?", evo.Destructive())`,
			Remediation:     "Add evo.Destructive() to any Confirm question containing delete/remove/trash/retire/force",
			RelatedGuidance: []string{"interactive", "common-api"},
			VerificationIDs: []string{"CONFIRM-002"},
			Since:           "0.7.0",
			Certainty:       "heuristic",
		},
		{
			ID:        "CON-002",
			Category:  "CON",
			Severity:  "warning",
			Invariant: "a failure summary is printed once, from Conclusion, not hand-assembled from a collected list",
			Why:       "fmt/out.Print(strings.Join(failures, ...)) duplicates the exact summary Conclusion already owns and can drift from the glyphs/exit code the ledger shows.",
			BadCode:   `out.Println(strings.Join(failures, "\n")) // duplicates Conclusion`,
			GoodCode: `for _, f := range failures {
  out.Item(f.Name).Fail(f.Reason)
}
// Conclusion renders the one summary; add Next(evo.Label(...)) for guidance`,
			Remediation:     "Resolve each failure on its own Item/Task and let Conclusion summarize; use Next for follow-up guidance",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"CON-002"},
			Since:           "0.7.0",
			Certainty:       "heuristic",
		},
		{
			ID:              "FP-004",
			Category:        "FP",
			Severity:        "warning",
			Invariant:       "a Phase string names the domain object in motion, not a generic placeholder",
			Why:             "\"starting\"/\"working\"/\"running\"/\"please wait\" tells the user nothing changed since the last frame — the same illegible-spinner defect FP-003 covers for a silent subprocess.",
			BadCode:         `task.Phase("working")`,
			GoodCode:        `task.Phase("scanning ~/Developer/Personal/zq")`,
			Remediation:     "Name the object the task is currently acting on in the Phase string",
			RelatedGuidance: []string{"first-paint", "tasks"},
			VerificationIDs: []string{"FP-004"},
			Since:           "0.7.0",
			Certainty:       "heuristic",
		},
		{
			ID:              "MCP-021",
			Category:        "MCP",
			Severity:        "error",
			Invariant:       "agents stop only when recheck_required is false",
			Why:             "Stopping while recheck_required is true leaves known defects unfixed.",
			BadCode:         `// agent: one review call then ship`,
			GoodCode:        `// loop: review → repair → review until recheck_required=false`,
			Remediation:     "Loop review until clean; use harness.RunRepairLoop",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"MCP-021", "MCP-022", "MCP-049"},
			Since:           "0.5.0",
			Certainty:       "deterministic",
		},
		{
			ID:        "API-001",
			Category:  "API",
			Severity:  "warning",
			Invariant: "the minimal Item happy path (For/Item/OK/Block/Finish) compiles with no Config struct",
			Why:       "Requiring a Config struct for the common single-item check adds ceremony that discourages the minimal, correct spelling.",
			BadCode: `out := evo.New(evo.Config{}) // empty struct passed for no reason
it := out.Item("disk space")
it.OK()`,
			GoodCode: `out := evo.New()
it := out.Item("disk space")
it.OK()`,
			Remediation:     "Use evo.New() with no Config for the minimal happy path; add Config only to override stream or behavior defaults",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"API-001"},
			Since:           "0.1.0",
			Certainty:       "heuristic",
			Detection:       "guidance", // no cheap detector: an empty/default Config literal is not distinguishable from an intentional one by AST alone
		},
		{
			ID:        "DOM-006",
			Category:  "DOM",
			Severity:  "warning",
			Invariant: "Item.OK/Block is a direct legal terminal transition; explicit Start is not required",
			Why:       "An Item is pending until it resolves; calling Start first is redundant ceremony and risks a spinner flash for a transition that finishes instantly.",
			BadCode: `it := out.Item("disk space")
it.Start()
it.OK()`,
			GoodCode: `it := out.Item("disk space")
it.OK()`,
			Remediation:     "Call OK/Block/BlockedBy directly; remove the explicit Start call",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"DOM-006"},
			Since:           "0.1.0",
			Certainty:       "deterministic",
		},
		{
			ID:              "DOM-007",
			Category:        "DOM",
			Severity:        "warning",
			Invariant:       "Item.Block(summary, options...) creates exactly one anonymous Problem; BlockedBy is for pre-built, multiple Problems",
			Why:             "Hand-building a single-element Problem slice for BlockedBy duplicates what Block already does automatically and can drift from the anonymous-problem shape Block guarantees.",
			BadCode:         `it.BlockedBy(evo.Problem{Summary: "disk full"})`,
			GoodCode:        `it.Block("disk full")`,
			Remediation:     "Use Block(summary, options...) for a single blocker; reserve BlockedBy for multiple pre-built Problems",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"DOM-007"},
			Since:           "0.1.0",
			Certainty:       "heuristic",
			Detection:       "guidance", // no cheap detector: distinguishing a legitimate single-Problem BlockedBy call from misuse needs call-site intent, not AST shape
		},
		{
			ID:        "DOM-016",
			Category:  "DOM",
			Severity:  "warning",
			Invariant: "Task.Phase without a prior Start activates the task directly into running, indeterminate state",
			Why:       "Requiring Start before Phase is the same redundant ceremony API-006 already forbids for Done; Phase alone carries enough information to activate the task.",
			BadCode: `t := out.Task("scan")
t.Start()
t.Phase("walking")`,
			GoodCode: `t := out.Task("scan")
t.Phase("walking")`,
			Remediation:     "Call Phase directly; do not call Start first — see API-006 for the general no-Start-needed rule",
			RelatedGuidance: []string{"tasks", "common-api"},
			VerificationIDs: []string{"DOM-016"},
			Since:           "0.1.0",
			Certainty:       "heuristic",
			Detection:       "guidance", // API-006 already carries the detector for the shared Start-then-X shape; DOM-016 documents the resulting state
		},
		{
			ID:        "DOM-017",
			Category:  "DOM",
			Severity:  "warning",
			Invariant: "Task.Progress(completed, total) stores absolute values, never a delta",
			Why:       "Passing a delta to Progress instead of Advance silently corrupts the absolute count — repeated Progress(1, total) calls read as stuck at 1, not incrementing.",
			BadCode: `for range items {
  t.Progress(1, total) // resets to 1 every call instead of incrementing
}`,
			GoodCode: `t.Progress(0, total) // seal indeterminate -> determinate once
for range items {
  t.Advance(1) // increments the prior absolute value
}`,
			Remediation:     "Use Progress(completed, total) only for absolute values; use Advance(delta) to increment",
			RelatedGuidance: []string{"tasks"},
			VerificationIDs: []string{"DOM-017"},
			Since:           "0.1.0",
			Certainty:       "heuristic",
			Detection:       "guidance", // no cheap detector: a literal "1" argument is not distinguishable from a genuine absolute count by AST alone
		},
		{
			ID:              "LOG-001",
			Category:        "LOG",
			Severity:        "warning",
			Invariant:       "log level markers render as a stable uppercase bracketed tag ([DEBUG], [WARN], [ERROR])",
			Why:             "A hand-formatted or lowercase level prefix breaks golden-stable log parsing and reads inconsistently against every other line evo emits.",
			BadCode:         `fmt.Fprintf(w, "warn: %s\n", msg)`,
			GoodCode:        `// route through evo's logging path (slog via SlogHandler, or out.Println) — it renders "[WARN] %s" itself`,
			Remediation:     "Let evo's logging renderer format the level tag; never hand-assemble a level prefix",
			RelatedGuidance: []string{"streams"},
			VerificationIDs: []string{"LOG-001"},
			Since:           "0.1.0",
			Certainty:       "heuristic",
			Detection:       "guidance", // no cheap detector: a hand-formatted level string is a plain fmt call, indistinguishable from ordinary text by AST alone
		},
		{
			ID:              "OUT-001",
			Category:        "OUT",
			Severity:        "warning",
			Invariant:       "the final human report writes through the configured human writer; transient live-region output never corrupts it",
			Why:             "Printing the final report through a different writer than Config wires, or interleaving it with live-region redraws, can duplicate or garble the report the human reads at exit.",
			BadCode:         `fmt.Println(finalReportText) // bypasses out's configured human writer`,
			GoodCode:        `out.Println(finalReportText) // routed through the same managed writer as everything else`,
			Remediation:     "Route the final report through the same Output writer as everything else; never fmt.Print it separately",
			RelatedGuidance: []string{"streams"},
			VerificationIDs: []string{"OUT-001"},
			Since:           "0.3.0",
			Certainty:       "heuristic",
			Detection:       "guidance", // no cheap detector: which writer the "final report" logically belongs to is a call-site judgment, not an AST pattern
		},
		{
			ID:        "OUT-003",
			Category:  "OUT",
			Severity:  "error",
			Invariant: "progress and live-UI bytes never reach stdout while a data projection (FormatData) is active",
			Why:       "A data command's stdout is a machine payload contract; any progress byte on stdout corrupts a JSON/line consumer downstream.",
			BadCode: `out := evo.New(evo.Config{Stdout: os.Stdout})
out.FormatData(...) // no Stderr configured: progress/UI also target Stdout`,
			GoodCode: `out := evo.New(evo.Config{Stdout: os.Stdout, Stderr: os.Stderr})
out.FormatData(...) // progress/UI route to Stderr; only the payload reaches Stdout`,
			Remediation:     "Configure Stderr alongside Stdout when using FormatData/ResultWriter; never write progress bytes to stdout by hand",
			RelatedGuidance: []string{"streams"},
			VerificationIDs: []string{"OUT-003"},
			Since:           "0.3.0",
			Certainty:       "heuristic",
			Detection:       "guidance", // no cheap detector: whether a given write targets the data channel vs. progress is a runtime routing question
		},
		{
			ID:              "OUT-004",
			Category:        "OUT",
			Severity:        "error",
			Invariant:       "Plain mode emits no ANSI escape or cursor-control bytes",
			Why:             "A hand-rolled escape sequence written outside evo's managed writer survives Plain mode and corrupts non-TTY/CI output that Plain exists to keep clean.",
			BadCode:         `fmt.Fprint(os.Stdout, "\x1b[32mok\x1b[0m") // raw ANSI bypasses Plain mode`,
			GoodCode:        `out.Println("ok") // evo suppresses ANSI automatically under Plain/NoColor or off-TTY`,
			Remediation:     "Never write raw ANSI/cursor sequences directly; let evo's managed writers decide based on Plain/NoColor/TTY detection",
			RelatedGuidance: []string{"streams"},
			VerificationIDs: []string{"OUT-004"},
			Since:           "0.1.0",
			Certainty:       "heuristic",
			Detection:       "guidance", // no cheap detector: a raw fmt.Fprint call with an escape-sequence string literal isn't reliably distinguishable from other formatted output
		},
		{
			ID:        "SEC-006",
			Category:  "SEC",
			Severity:  "warning",
			Invariant: "displayed shell/command arguments are quoted so argv boundaries survive presentation",
			Why:       "Naively space-joining a []string for display can misrepresent argv boundaries — an argument containing a space reads as two arguments — which misleads a human approving a destructive action.",
			BadCode:   `task.Phase(strings.Join(args, " ")) // "rm -rf my file.txt" reads as 4 words, not 3 args`,
			GoodCode: `quoted := make([]string, len(args))
for i, a := range args {
  quoted[i] = strconv.Quote(a) // preserves argv boundaries even when an arg contains a space
}
task.Phase(strings.Join(quoted, " "))`,
			Remediation:     "Quote each argument individually before joining for display; never join raw argv with bare spaces",
			RelatedGuidance: []string{"security"},
			VerificationIDs: []string{"SEC-006"},
			Since:           "0.1.0",
			Certainty:       "heuristic",
			Detection:       "guidance", // no cheap detector: strings.Join(args, " ") is a common, mostly-safe pattern; flagging it requires knowing args came from a shell command
		},
		{
			ID:              "TERM-006",
			Category:        "TERM",
			Severity:        "warning",
			Invariant:       "a debug/log line written during live UI erases the region, appends the line, and redraws — never interleaves raw",
			Why:             "A log line written straight to the terminal while a spinner/live region is open tears the frame and corrupts the display until the next redraw.",
			BadCode:         `fmt.Fprintln(os.Stderr, "[DEBUG] cache miss") // interleaves under the live spinner`,
			GoodCode:        `out.Println("[DEBUG] cache miss") // clears the region, writes, redraws atomically`,
			Remediation:     "Route debug/log lines through evo.Println/Print/Printf (or slog via SlogHandler) instead of writing to the raw stream while a live region is open",
			RelatedGuidance: []string{"interactive"},
			VerificationIDs: []string{"TERM-006"},
			Since:           "0.2.0",
			Certainty:       "heuristic",
			Detection:       "guidance", // no cheap detector: whether a live region is open at a given fmt call site is a runtime property, not visible in source
		},
		{
			ID:              "TXT-007",
			Category:        "TXT",
			Severity:        "error",
			Invariant:       "ESC/CSI byte sequences embedded in any caller-supplied text field are neutralized before rendering",
			Why:             "A malicious or fuzzed ESC/CSI sequence embedded in a name/detail/phase string must never survive into the terminal write; evo's sanitize layer is the single point that guarantees this, so nothing should bypass it with a raw write of untrusted text.",
			BadCode:         `fmt.Fprint(w, rawUserText) // bypasses evo's sanitize layer entirely`,
			GoodCode:        `it := out.Item(rawUserText) // evo sanitizes ESC/CSI internally before it reaches the terminal`,
			Remediation:     "Never bypass evo's managed Item/Task/Print* entry points with a raw write of untrusted text; sanitization only runs on the managed path",
			RelatedGuidance: []string{"security"},
			VerificationIDs: []string{"TXT-007", "SEC-001"},
			Since:           "0.1.0",
			Certainty:       "heuristic",
			Detection:       "guidance", // no cheap detector: a raw write of "untrusted" text isn't distinguishable from a raw write of trusted text by AST alone
		},
	}
}

// Explain returns a rule by ID (MCP-027 full payload).
func Explain(id string) (Rule, bool) {
	for _, r := range All() {
		if r.ID == id {
			return r, true
		}
	}
	return Rule{}, false
}

// IDs returns the stable set of rule identifiers (MCP-028 compatibility surface).
func IDs() []string {
	all := All()
	out := make([]string, len(all))
	for i, r := range all {
		out[i] = r.ID
	}
	return out
}
