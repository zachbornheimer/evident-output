// Package review — hand-assembled failure text: embedding retained evidence
// in a Fail/Block summary, or printing a joined list where Conclusion
// already owns the summary.
package review

import "regexp"

// failCaptureTextPattern is EV-001: a Fail/Block summary built with
// fmt.Sprintf("...%s...", capture.Text()) (or string concatenation) folds
// the retained evidence ring straight into the summary the row already
// shows — the auto-attach then renders the exact same text a second time as
// evidence underneath it (user-13-problems.md Problem 7). Matches any
// receiver's .Text()/.Tail() call appearing as a Fail/Block argument, not
// just a variable literally named "capture" — the misuse is the method call
// shape, not the identifier.
var failCaptureTextPattern = regexp.MustCompile(`\.(?:Fail|Block)\([^)]*\.(?:Text|Tail)\(\)[^)]*\)`)

// detectFailEmbeddedEvidenceText flags EV-001's anti-pattern.
func detectFailEmbeddedEvidenceText(filename, src string) []Finding {
	var findings []Finding
	for _, m := range failCaptureTextPattern.FindAllStringIndex(src, -1) {
		findings = append(findings, Finding{
			RuleID:     "EV-001",
			Message:    "Fail/Block argument calls .Text()/.Tail() on the retained evidence ring — that text is already auto-attached as a separate evidence line, so embedding it in the summary too duplicates it",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: `pass context via a %w-wrapped error returned from Define instead — e.g. return fmt.Errorf("install dependencies: %w", err) — and let the auto-attach carry the retained tail`,
		})
	}
	return findings
}

// printJoinPattern matches a Print/Println/Printf call fed a joined list —
// the hand-assembled failure summary evo-rec.md's Conclusion already owns.
var printJoinPattern = regexp.MustCompile(`\.(Print|Println|Printf)\(\s*strings\.Join\(`)

// detectHandAssembledFailureSummary flags a Print* call whose argument joins
// a collected list — that summary duplicates Conclusion and can drift from
// the glyphs/exit code the ledger already shows.
func detectHandAssembledFailureSummary(filename, src string) []Finding {
	var findings []Finding
	for _, m := range printJoinPattern.FindAllStringIndex(src, -1) {
		findings = append(findings, Finding{
			RuleID:     "CON-002",
			Message:    "printing a joined list duplicates the Conclusion summary; resolve each item on its own Item/Task instead",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: "replace with one Item/Task per entry, and Next(evo.Label(...)) for follow-up guidance",
		})
	}
	return findings
}
