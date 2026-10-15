// Package review — hand-assembled failure text.
package review

import "regexp"

// printJoinPattern matches a direct Print* call, or a Fprint* call to the
// run's free text writer (Run.Text), fed a joined list.
var printJoinPattern = regexp.MustCompile(`\.(?:Print|Println|Printf)\(\s*strings\.Join\(|\.(?:Fprint|Fprintln)\(\s*[\w.]*\.Text\(\)\s*,\s*strings\.Join\(`)

// detectHandAssembledFailureSummary flags a print of a joined list — that
// summary duplicates Conclusion and can drift from the outcomes and exit
// code the ledger already shows.
func detectHandAssembledFailureSummary(filename, src string) []Finding {
	var findings []Finding
	for _, m := range printJoinPattern.FindAllStringIndex(src, -1) {
		findings = append(findings, Finding{
			RuleID:     "CON-002",
			Message:    "printing a joined list duplicates the Conclusion summary; resolve each item on its own Task instead",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: "replace with one Task per entry, each returning evo.Fail(...), and evo.Next(evo.Action{Label: ...}) for follow-up guidance",
		})
	}
	return findings
}
