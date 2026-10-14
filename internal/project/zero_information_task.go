package project

import "github.com/zachbornheimer/evident-output/v2/internal/record"

// IsZeroInformationTask reports whether t alone, ignoring its ledger and its
// neighbours, would add nothing to a human reader: a Succeeded Task that changed
// nothing (NoWork or AlreadySatisfied), was not invented by the library, and
// carries no information field (see zeroInformationFields in the test,
// which fails when a record.TaskSnapshot field is added without deciding whether
// it is information). Where such a row may actually be hidden is
// presentation policy (render's zero-information rule).
func IsZeroInformationTask(t record.TaskSnapshot) bool {
	if !t.State.Succeeded() || t.Synthetic() {
		return false
	}
	if t.DefineRan {
		return false
	}
	return t.Summary == "" && t.Phase == "" && t.Progress.Total == 0 && t.Progress.Completed == 0 &&
		len(t.Problems) == 0 && len(t.Warnings) == 0 && len(t.Facts) == 0 &&
		len(t.Skipped) == 0 && len(t.Kept) == 0 && len(t.Actions) == 0 &&
		len(t.Verification) == 0
}

// IsProvenNoOpTask reports whether t is a zero-information Task whose no-op
// Verify proved (AlreadySatisfied), as opposed to one that merely returned
// without doing anything.
func IsProvenNoOpTask(t record.TaskSnapshot) bool {
	return IsZeroInformationTask(t) && t.State == record.Satisfied
}
