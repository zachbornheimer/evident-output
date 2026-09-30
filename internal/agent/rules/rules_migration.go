package rules

// MigrationRow is one machine-readable version-transition entry (spec §59):
// a stale/legacy shape and the current replacement, keyed by the rule that
// detects the stale shape where one exists (empty for a shape no detector
// currently flags).
type MigrationRow struct {
	From    string `json:"from"`
	To      string `json:"to"`
	RuleID  string `json:"rule_id,omitempty"`
	Since   string `json:"since"`
	Notes   string `json:"notes,omitempty"`
	Removed bool   `json:"removed,omitempty"` // From no longer compiles/exists as of Since
}

// Migrations returns the version-transition table an MCP upgrade assistant
// (spec §58/§59) can consume without parsing prose: given an observed
// call-site shape, look up its row for the mechanical replacement.
func Migrations() []MigrationRow {
	return []MigrationRow{
		{
			From:  "legacy presentation-only Task usage",
			To:    "scheduled Task/Group/Sequence",
			Since: "1.0.0",
			Notes: "a Task used only to print status, with no Define submitting work, becomes a scheduled child of Group/Sequence",
		},
		{
			From:   "mutating Evidence callback (task.Evidence(\"write\", func() error { ... }))",
			To:     "mutation in Define (evo.Effect / evo.File)",
			RuleID: "EVO-EVIDENCE-001",
			Since:  "1.0.0",
			Notes:  "a callback that mutates state belongs in Define; capture-meaning Evidence* was removed in 1.1 (Capture is the retained sink)",
		},
		{
			From:   "manual os.WriteFile + os.Chmod + read-only Evidence",
			To:     "evo.File declarative tracked operation",
			RuleID: "EVO-FILE-001",
			Since:  "1.0.0",
			Notes:  "one FileSpec{Path, Contents, Mode, Basis} call replaces the write/chmod/check boilerplate where semantics match",
		},
		{
			// EVO-EVIDENCE-002 would be spec §57's detector for this shape;
			// no such rule exists yet, so left uncited rather than claiming
			// a rule ID that does not resolve.
			From:  "named Evidence used only for common file state",
			To:    "evo.File's own tracked record (no extra Capture)",
			Since: "1.0.0",
			Notes: "a named capture that only restates a file's own tracked state (path/mode/hash) duplicates evo.File's own record instead of adding new information",
		},
		{
			From:  "manual counters (hand-incremented progress totals)",
			To:    "Group/Sequence derived progress",
			Since: "1.0.0",
			Notes: "one Task per item under Group/Sequence gives correct progress without a hand-maintained counter",
		},
		{
			// EVO-WIRE-001 is spec §57's detector for this shape; not yet
			// implemented in this rule set, so left uncited rather than
			// claiming a rule ID that does not resolve.
			From:  "manual JSON struct marshal of Snapshot/Result",
			To:    "versioned stable encoder (FormatJSON/FormatJSONL)",
			Since: "1.0.0",
			Notes: "internal Snapshot/Result types are not public API; the versioned encoder owns the wire schema and its version bump",
		},
		{
			From:  "hand-built skip text (\"skipped: already done\")",
			To:    "Done resolution / AlreadySatisfied",
			Since: "1.0.0",
			Notes: "already-satisfied is a Task resolution the scheduler renders consistently, not a caller-formatted string",
		},
		{
			From:    "evo.MainWith(...)",
			To:      "os.Exit(evo.Main(run)) where run(ctx context.Context) error",
			RuleID:  "API-018",
			Since:   "1.0.0",
			Removed: true,
			Notes:   "MainWith was removed in 1.0; Main derives the exit code, os.Exit applies it",
		},
		{
			From:    "Group.Each(...) / Sequence.Each(...)",
			To:      "one Task per item under Group(...)/Sequence(...)",
			RuleID:  "PROG-001",
			Since:   "1.0.0",
			Removed: true,
			Notes:   "Each was removed in 1.0; each loop item becomes its own named child Task",
		},
		{
			From:    "task.Blockf(...)",
			To:      "task.Block(summary, opts...)",
			RuleID:  "API-032",
			Since:   "1.1.0",
			Removed: true,
			Notes:   "Blockf was removed in 1.1; Block is a statement and takes ProblemOptions, not a format string",
		},
		{
			From:    "task.Failf(...)",
			To:      "task.Fail(summary, opts...)",
			RuleID:  "API-032",
			Since:   "1.1.0",
			Removed: true,
			Notes:   "Failf was removed in 1.1; Fail is a statement. Inside Define, return fmt.Errorf and let Define resolve; attach user-facing text with evo.Detail",
		},
		{
			From:    "task.Warn / evo.Warn / Output.Warn",
			To:      "task.Problem(summary, evo.Severity(evo.SeverityWarning))",
			RuleID:  "API-032",
			Since:   "1.1.0",
			Removed: true,
			Notes:   "Warn was removed in 1.1; a warning is a Problem at SeverityWarning and does not resolve the Task",
		},
		{
			From:    "task.Step(completed, total, item)",
			To:      "task.Progress(completed, total).Doing(item)",
			RuleID:  "API-032",
			Since:   "1.1.0",
			Removed: true,
			Notes:   "Step was removed in 1.1; Progress sets the counts and Doing names the live item",
		},
		{
			From:    "task.Kept(reason)",
			To:      "task.Skipped(evo.Reason(...)) for a policy-excluded candidate; Fact/Summary for a kept count",
			RuleID:  "API-032",
			Since:   "1.1.0",
			Removed: true,
			Notes:   "Kept was removed in 1.1; policy exclusion is Skipped. Renderer copy like \"! kept N\" is not a live method",
		},
		{
			From:    "task.Evidence() / EvidenceOption / EvidenceStream* / MaxEvidenceBytes",
			To:      "task.Capture() / CaptureOption / CaptureStream* / MaxCaptureBytes",
			RuleID:  "API-032",
			Since:   "1.1.0",
			Removed: true,
			Notes:   "capture-meaning Evidence* was removed in 1.1; Capture is the retained sink. EvidencePhase and TaskEvidence stay (satisfaction-meaning). Mutating Evidence(\"name\", fn) is EVO-EVIDENCE-001",
		},
		{
			From:    "evo.Option / []evo.Option / Config.Options / To / AlsoWrite / Plain / NoColor / Stdin / DryRun / VisibilityDelay / Diagnostics / Title / ResultStream / Terminal / Clock / MaxFrameRate / Width / Redact / Runner / MaxEntities / MaxEvents / Strict / Glyphs / DataProjection / ExternalProjection / DebugLevel / DebugAddSource / DebugHistory / DebugPane",
			To:      "Config fields (Stdout, Stderr, Result, Stdin, Title, Terminal, Clock, MaxFrameRate, Width, Redactor, ProcessRunner, MaxEntities, MaxEvents, Glyphs, Plain, DryRun, Strict, Color: ColorNever, VisibilityDelay, Format: FormatData/FormatExternal, Debug: DebugConfig{...}); tee with io.MultiWriter on Stdout",
			RuleID:  "API-032",
			Since:   "1.1.0",
			Removed: true,
			Notes:   "Option constructors were removed in 1.1; Config fields are the constructor",
		},
		{
			From:    "EncodeJSON / EncodeJSONL / EncodeEventJSON / ConclusionJSON / EventJSON / JSONDocument / JSONAction / JSONChanges / JSONCollection / JSONCommand / JSONEffectRecord / JSONMessage / JSONOutputMeta / JSONPlan / JSONProblem / JSONProgress / JSONSchemaVersion / JSONTask",
			To:      "WriteJSON / FormatJSON / FormatJSONL",
			RuleID:  "API-032",
			Since:   "1.1.0",
			Removed: true,
			Notes:   "JSON encoder types and EncodeJSON* were removed in 1.1; WriteJSON writes the run document and FormatJSON/FormatJSONL select Config.Format",
		},
		{
			From:    "Failure.Next / Failure.NextCommand",
			To:      "Next / NextCommand ProblemOptions on Fail or Block",
			RuleID:  "API-032",
			Since:   "1.1.0",
			Removed: true,
			Notes:   "Failure.Next was removed in 1.1; pass evo.Next(...) as an option to Fail or Block",
		},
		{
			From:    "TaskHandle.Next / TaskHandle.NextCommand / Output.Next / Output.NextCommand",
			To:      "evo.Next / evo.NextCommand options on Problem, Fail, or Block",
			RuleID:  "API-032",
			Since:   "1.1.0",
			Removed: true,
			Notes:   "A remedy belongs to the Problem it explains (ZYS-1182); a remedy with no failure becomes a warning-severity Problem on the Task that motivates it",
		},
		{
			From:    "ForSkip / OnTask / ReasonOption / ErrReasonSkipOnly / ErrReasonWrongTask",
			To:      "Reason(name) plus Skipped",
			RuleID:  "API-032",
			Since:   "1.1.0",
			Removed: true,
			Notes:   "ForSkip, OnTask, ReasonOption, and ErrReason* were removed in 1.1; name the skip with Reason and resolve with Skipped",
		},
		{
			From:    "EventSchemaVersion",
			To:      "wire document schema version (not an evo export)",
			RuleID:  "API-032",
			Since:   "1.1.0",
			Removed: true,
			Notes:   "EventSchemaVersion was removed in 1.1; the version lives on the wire document",
		},
	}
}
