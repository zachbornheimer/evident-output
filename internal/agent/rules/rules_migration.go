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
			Notes: "a Task used only to print status, with no Define/mutation verb submitting work, becomes a scheduled child of Group/Sequence",
		},
		{
			// EVO-EVIDENCE-001 is spec §57's detector for this shape; it
			// ships in a parallel slice of this work and is intentionally
			// not cited here until that rule lands in coreRules/fileAndExecRules.
			From:  "mutating Evidence callback (task.Evidence(\"write\", func() error { ... }))",
			To:    "mutation in Define",
			Since: "1.0.0",
			Notes: "Evidence is read-only; a callback that mutates state belongs in Define or a mutation verb, not Evidence",
		},
		{
			From:   "manual os.WriteFile + os.Chmod + read-only Evidence",
			To:     "evo.File declarative tracked operation",
			RuleID: "EVO-FILE-001",
			Since:  "1.0.0",
			Notes:  "one FileSpec{Path, Contents, Mode, Basis} call replaces the write/chmod/check boilerplate where semantics match",
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
	}
}
