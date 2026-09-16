package review

import "regexp"

// uiFactLinePattern matches a Printf whose literal format string is a
// "Label: %verb" line — the shape of a routine key/value observation
// hand-printed instead of recorded as a Fact (EVO-UI-001).
var uiFactLinePattern = regexp.MustCompile(`\.Printf\(\s*(?:fmt\.Sprintf\()?\s*"([A-Za-z][\w ./-]{0,60}):\s*%[a-zA-Z]`)

// detectFactPrintedAsUIText flags a manually printed "label: value" line
// that duplicates task.Fact — the renderer/JSON both already derive from
// Facts, and a hand-printed line is unstructured text neither can rely on.
func detectFactPrintedAsUIText(filename, src string) []Finding {
	var findings []Finding
	for _, m := range uiFactLinePattern.FindAllStringSubmatchIndex(src, -1) {
		label := src[m[2]:m[3]]
		findings = append(findings, Finding{
			RuleID:     "EVO-UI-001",
			Severity:   "warning",
			Message:    `manually printed "` + label + `: ..." line duplicates task.Fact; a Fact is derived and projected consistently across renderer and JSON`,
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: `replace with task.Fact("` + label + `", value)`,
		})
	}
	return findings
}

// uiSuccessGlyphPattern matches a Print-family call whose literal contains a
// hand-picked success glyph or word (EVO-UI-002) — Task.Done already renders
// the passing state.
var uiSuccessGlyphPattern = regexp.MustCompile(`\.(?:Print|Printf|Println)\(\s*(?:fmt\.Sprintf\()?\s*"[^"]*(?:✓|(?i:verified|passed))[^"]*"`)

// detectPassingVerificationPrinted flags a hand-printed success/verified
// line, which duplicates the glyph Task.Done already renders on the
// passing path and drifts from it under Plain/JSON/verbosity modes.
func detectPassingVerificationPrinted(filename, src string) []Finding {
	var findings []Finding
	for _, loc := range uiSuccessGlyphPattern.FindAllStringIndex(src, -1) {
		findings = append(findings, Finding{
			RuleID:     "EVO-UI-002",
			Severity:   "warning",
			Message:    "manually printed success/verified line duplicates the glyph task.Done already renders on the passing path",
			File:       filename,
			Line:       lineAt(src, loc[0]),
			Suggestion: "delete the manual success line; let task.Done() render the passing state",
		})
	}
	return findings
}

// uiHandBuiltProgressPattern matches a Print-family call whose literal
// hand-assembles an "N/M" or "N of M" progress count (EVO-UI-003) instead
// of letting Task.Progress derive it from real state.
var uiHandBuiltProgressPattern = regexp.MustCompile(`\.(?:Print|Printf|Println)\(\s*(?:fmt\.Sprintf\()?\s*"[^"]*%d\s*(?:/|of)\s*%d`)

// detectHandBuiltProgressText flags hand-assembled "N/M done" text, which
// duplicates counts evo already derives from Task/Group/Sequence state and
// can silently disagree with them.
func detectHandBuiltProgressText(filename, src string) []Finding {
	var findings []Finding
	for _, loc := range uiHandBuiltProgressPattern.FindAllStringIndex(src, -1) {
		findings = append(findings, Finding{
			RuleID:     "EVO-UI-003",
			Severity:   "warning",
			Message:    "hand-built \"N/M\" progress text duplicates counts evo already derives from Task/Group state",
			File:       filename,
			Line:       lineAt(src, loc[0]),
			Suggestion: "replace with task.Progress(completed, total) or the owning Group/Sequence summary",
		})
	}
	return findings
}

// wireMarshalSnapshotPattern matches json.Marshal/MarshalIndent called
// directly on a .Snapshot()/.Result() expression (EVO-WIRE-001) — the
// internal shape marshaled as if it were the public wire contract.
var wireMarshalSnapshotPattern = regexp.MustCompile(`json\.(Marshal|MarshalIndent)\(\s*([\w.]+)\.(Snapshot|Result)\(\)`)

// detectMarshalOfInternalSnapshot flags json.Marshal(x.Snapshot()) (or
// .Result()), which bypasses the sanctioned, versioned JSON encoder and
// leaks undocumented internal field names/shape to consumers.
func detectMarshalOfInternalSnapshot(filename, src string) []Finding {
	var findings []Finding
	for _, m := range wireMarshalSnapshotPattern.FindAllStringSubmatchIndex(src, -1) {
		fn := src[m[2]:m[3]]
		recv := src[m[4]:m[5]]
		method := src[m[6]:m[7]]
		call := recv + "." + method + "()"
		findings = append(findings, Finding{
			RuleID:     "EVO-WIRE-001",
			Severity:   "error",
			Message:    "json." + fn + " marshals the internal " + method + " directly; use the sanctioned JSON encoder instead",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: "replace json." + fn + "(" + call + ") with render.EncodeJSON(" + call + ")",
		})
	}
	return findings
}

// jsonEncoderToStdoutPattern matches the canonical JSON/JSONL-to-stdout
// idiom; jsonStdoutHumanTextPattern matches an ordinary human print
// reaching stdout in the same file (EVO-WIRE-003).
var (
	jsonEncoderToStdoutPattern = regexp.MustCompile(`json\.NewEncoder\(\s*os\.Stdout\s*\)\.Encode\(`)
	jsonStdoutHumanTextPattern = regexp.MustCompile(`\bfmt\.(?:Print|Printf|Println)\(|fmt\.Fprint(?:f|ln)?\(\s*os\.Stdout\s*,`)
)

// detectJSONStdoutMixedWithHumanText flags a file that encodes JSON/JSONL
// to stdout and also writes ordinary human text to stdout — a JSON/JSONL
// consumer cannot parse a stream with a human line spliced into it.
func detectJSONStdoutMixedWithHumanText(filename, src string) []Finding {
	if !jsonEncoderToStdoutPattern.MatchString(src) {
		return nil
	}
	loc := jsonStdoutHumanTextPattern.FindStringIndex(src)
	if loc == nil {
		return nil
	}
	return []Finding{{
		RuleID:     "EVO-WIRE-003",
		Severity:   "error",
		Message:    "human text written to stdout in a file that also encodes JSON/JSONL to stdout; a machine consumer cannot parse the mixed stream",
		File:       filename,
		Line:       lineAt(src, loc[0]),
		Suggestion: "route human presentation to Stderr (evo.FormatData) so stdout carries only the JSON/JSONL payload",
	}}
}
