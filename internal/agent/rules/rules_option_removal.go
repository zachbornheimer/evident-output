package rules

// optionRemoval is one Option constructor removed in 1.1 (E-122 lane F2:
// Config.Options and the exported Option type only duplicated Config
// fields) and the Config field/value it collapses onto.
type optionRemoval struct {
	// RuleID is the MCP migration rule for this one removed name.
	RuleID string
	// Name is the removed evo.<Name> constructor (or DebugPaneOption
	// constructor for the four debug-pane knobs).
	Name string
	// BadArg is a compiling argument for evo.<Name>(BadArg) in BadCode.
	BadArg string
	// GoodField is the Config field assignment GoodCode sets instead
	// ("Stdout: w" for evo.To(w), removed in 1.1; "Plain: true" for
	// evo.Plain(), also removed in 1.1; ...).
	GoodField string
	// Why explains the specific field mapping; the shared preamble (the
	// []Option escape hatch forked Config's ordinary construction path)
	// is prepended by optionRemovalRules.
	Why string
}

// optionRemovals is the one table of removed Option constructors this file
// and CHANGELOG.md / docs/migration/1.1.md's table both name — every entry
// here must also appear in internal/retired's E-122 lane F2 block so the
// contract check, the stale-docs scan, and this rule catalog cannot drift.
var optionRemovals = []optionRemoval{
	{RuleID: "API-130", Name: "AlsoWrite", BadArg: "&mirror", GoodField: "AlsoWrite: []io.Writer{&mirror}", Why: "AlsoWrite mirrored bytes to an extra writer; Config.AlsoWrite is that same []io.Writer, a new field (there was no prior Config equivalent)."},
	{RuleID: "API-131", Name: "Clock", BadArg: "fixedClock", GoodField: "Clock: fixedClock", Why: "Clock injected a TimeSource; Config.Clock is the same facade seam."},
	{RuleID: "API-132", Name: "DataProjection", BadArg: "", GoodField: "Format: evo.FormatData", Why: "DataProjection reserved Stdout for the domain payload; Config.Format: FormatData already drives that same routing."},
	{RuleID: "API-133", Name: "DebugAddSource", BadArg: "", GoodField: "Debug: evo.DebugConfig{AddSource: true}", Why: "DebugAddSource resolved each record's call site; Config.Debug.AddSource is the same flag."},
	{RuleID: "API-134", Name: "DebugHistory", BadArg: "", GoodField: "Debug: evo.DebugConfig{View: evo.DebugPresentationHistory}", Why: "DebugHistory selected durable scrollback; Config.Debug.View is the same selector (and its own zero value)."},
	{RuleID: "API-135", Name: "DebugLevel", BadArg: "evo.LevelDebug", GoodField: "Debug: evo.DebugConfig{Level: evo.LevelDebug}", Why: "DebugLevel set the minimum debug level; Config.Debug.Level is the same field."},
	{RuleID: "API-136", Name: "DebugPane", BadArg: "evo.PaneHeight(3)", GoodField: "Debug: evo.DebugConfig{View: evo.DebugPresentationPane, PaneHeight: 3}", Why: "DebugPane only ever fanned its DebugPaneOption arguments (NewestFirst, OldestFirst, PaneHeight, PreserveDebugTail — kept, for embedders composing their own presentation layer) into the same four Config.Debug fields (View, PaneHeight, NewestFirst, PreserveAlways); ordinary callers now set those fields directly instead of building the Option through DebugPane."},
	{RuleID: "API-137", Name: "Diagnostics", BadArg: "&diag", GoodField: "Stderr: &diag", Why: "Diagnostics set the diagnostic writer, which was always Config.Stderr under the hood; there was never a second destination to route to."},
	{RuleID: "API-138", Name: "DryRun", BadArg: "", GoodField: "DryRun: true", Why: "DryRun declared the run a dry run; Config.DryRun is the same flag."},
	{RuleID: "API-139", Name: "ExternalProjection", BadArg: "", GoodField: "Format: evo.FormatExternal", Why: "ExternalProjection disabled inline rendering; Config.Format: FormatExternal already selects it."},
	{RuleID: "API-140", Name: "Glyphs", BadArg: "evo.GlyphsASCII", GoodField: "Glyphs: evo.GlyphsASCII", Why: "Glyphs selected the glyph capability profile; Config.Glyphs is the same field."},
	{RuleID: "API-141", Name: "MaxEntities", BadArg: "64", GoodField: "MaxEntities: 64", Why: "MaxEntities capped total items/tasks; Config.MaxEntities is the same limit."},
	{RuleID: "API-142", Name: "MaxEvents", BadArg: "64", GoodField: "MaxEvents: 64", Why: "MaxEvents capped the durable journal; Config.MaxEvents is the same limit."},
	{RuleID: "API-143", Name: "MaxFrameRate", BadArg: "30", GoodField: "MaxFrameRate: 30", Why: "MaxFrameRate capped live redraws per second; Config.MaxFrameRate is the same limit."},
	{RuleID: "API-144", Name: "NoColor", BadArg: "", GoodField: "Color: evo.ColorNever", Why: "NoColor forced color off; Config.Color: ColorNever is the same policy, one enum instead of a second boolean."},
	{RuleID: "API-145", Name: "Plain", BadArg: "", GoodField: "Plain: true", Why: "Plain disabled live frames; Config.Plain is the same flag."},
	{RuleID: "API-146", Name: "Redact", BadArg: "redactor", GoodField: "Redactor: redactor", Why: "Redact injected a Redactor; Config.Redactor is the same facade seam."},
	{RuleID: "API-147", Name: "ResultStream", BadArg: "&result", GoodField: "Result: &result", Why: "ResultStream set the domain-payload writer; Config.Result is the same field."},
	{RuleID: "API-148", Name: "Runner", BadArg: "runner", GoodField: "ProcessRunner: runner", Why: "Runner installed a ProcessRunner; Config.ProcessRunner is the same facade seam (spec §8.4)."},
	{RuleID: "API-149", Name: "Stdin", BadArg: "answer", GoodField: "Stdin: answer", Why: "Stdin injected the reader Confirm answers come from; Config.Stdin is the same field."},
	{RuleID: "API-150", Name: "Strict", BadArg: "", GoodField: "Strict: true", Why: "Strict made recorded misuse panic; Config.Strict is the same flag."},
	{RuleID: "API-151", Name: "Terminal", BadArg: "driver", GoodField: "Terminal: driver", Why: "Terminal injected a TerminalDriver; Config.Terminal is the same seam."},
	{RuleID: "API-152", Name: "Title", BadArg: `"repo-retire"`, GoodField: `Title: "repo-retire"`, Why: "Title set the conclusion subject; Config.Title is the ordinary field every ordinary Init call already uses."},
	{RuleID: "API-153", Name: "To", BadArg: "&buf", GoodField: "Stdout: &buf", Why: "To set the primary human writer; Config.Stdout is the same field."},
	{RuleID: "API-154", Name: "VisibilityDelay", BadArg: "0", GoodField: "VisibilityDelay: evo.Delay(0)", Why: "VisibilityDelay set the first-paint wait; Config.VisibilityDelay (a *time.Duration, built with evo.Delay) is the same field."},
	{RuleID: "API-155", Name: "Width", BadArg: "80", GoodField: "Width: 80", Why: "Width fixed the render width; Config.Width is the same field."},
}

// optionRemovalRules turns optionRemovals into one migration Rule per
// constructor removed in 1.1. Detection is guidance-only: the pre-existing
// structural rewrite for the whole []Option/Config.Options shape (also
// removed in 1.1) is API-032 (review_rec.go's recSurfaceDetector) — these
// entries exist so evident_output_explain and the catalog name the exact Config field for
// each individually-removed spelling, the way API-110..API-117 do for the
// Evidence* capture renames.
func optionRemovalRules() []Rule {
	const preamble = "Config already had a field for everything Option set. " +
		"Config.Options (the raw []Option escape hatch) and the exported Option " +
		"type were removed in 1.1 with no alias, since they were a second " +
		"spelling for the same knobs and forced Init's ordinary construction " +
		"path (stream/TTY/color inference, DryRun/Preview/Subject) to fork " +
		"around callers who set Options. "
	out := make([]Rule, 0, len(optionRemovals))
	for _, r := range optionRemovals {
		out = append(out, Rule{
			ID:              r.RuleID,
			MinDialect:      "1.1.0",
			Category:        "API",
			Severity:        SeverityWarning,
			Invariant:       "evo." + r.Name + " was removed in 1.1; set Config's own field instead",
			Why:             preamble + r.Why,
			BadCode:         "evo.Init(evo.Config{Options: []evo.Option{evo." + r.Name + "(" + r.BadArg + ")}})",
			GoodCode:        "evo.Init(evo.Config{" + r.GoodField + "})",
			Remediation:     "Replace evo." + r.Name + "(...) and its Config.Options wrapping (removed in 1.1) with Config{" + r.GoodField + "}",
			RelatedGuidance: []string{"common-api"},
			VerificationIDs: []string{"API-032", r.RuleID},
			Since:           "1.1.0",
			Certainty:       CertaintyDeterministic,
			Detection:       DetectionGuidance,
		})
	}
	return out
}

func init() { registerFamily(optionRemovalRules()) }
