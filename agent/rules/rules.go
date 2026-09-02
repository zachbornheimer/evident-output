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
			Invariant: "subprocess evidence uses Task.Capture, not DebugWriter",
			Why:       "DebugWriter is filtered by DebugLevel and is the wrong dialect for failure Detail tails.",
			BadCode: `dbg := out.DebugWriter()
run.Run(ctx, "brew", args, dbg)`,
			GoodCode: `output := task.Capture()
run.Run(ctx, "brew", args, output)
task.Fail("brew failed", evo.Cause(err), output.DetailTail())`,
			Remediation:     "Use task.Capture() + DetailTail on Fail",
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
