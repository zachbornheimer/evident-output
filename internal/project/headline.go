package project

import "github.com/zachbornheimer/evident-output/v2/internal/record"

// The words a Succeeded run's band uses for its tense instead of naming the
// Outcome: what changed, or what a dry run or a plan would change.
const (
	HeadlineChanged = "changed"
	HeadlinePlanned = "planned"
)

// ConclusionHeadline is the word the conclusion band names the run by: its Outcome in
// the Outcome's own word, except that a Succeeded run that planned or
// changed something reads in that tense.
func ConclusionHeadline(c record.Conclusion) string {
	if c.Outcome != record.OutcomeSucceeded {
		return string(c.Outcome)
	}
	switch {
	case c.DryRun || (len(c.Plans) > 0 && !c.Changed):
		return HeadlinePlanned
	case c.Changed:
		return HeadlineChanged
	}
	return string(c.Outcome)
}

// HeadlineColor is the color of the band that names headline.
func HeadlineColor(headline string) string {
	switch headline {
	case string(record.OutcomeSucceeded), string(record.OutcomeSatisfied), HeadlineChanged:
		return SGRGreen
	case HeadlinePlanned:
		return SGRBlue
	case string(record.OutcomeFailed), string(record.OutcomeRefused):
		return SGRRed
	case string(record.OutcomeCancelled), string(record.OutcomeExcluded):
		return SGRDim
	}
	return SGRCyan
}
