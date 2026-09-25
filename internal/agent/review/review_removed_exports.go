// Package review — API-120: exports the 1.1 freeze removed because they
// served no concept a caller needs (E-122).
package review

func init() {
	registerRemoved(
		removedName{
			rule: "API-120", name: "ForSkip",
			message:    "evo.ForSkip was removed in 1.1: a Reason has no usage constraints",
			suggestion: "delete the option: evo.Reason(name) takes only its name",
		},
		removedName{
			rule: "API-120", name: "OnTask",
			message:    "evo.OnTask was removed in 1.1: a Reason has no usage constraints",
			suggestion: "delete the option: evo.Reason(name) takes only its name",
		},
		removedName{
			rule: "API-120", name: "ReasonOption",
			message:    "evo.ReasonOption was removed in 1.1: a Reason has no usage constraints",
			suggestion: "delete it: evo.Reason(name) takes only its name",
		},
		removedName{
			rule: "API-120", name: "ErrReasonSkipOnly",
			message:    "evo.ErrReasonSkipOnly was removed in 1.1: a Reason has no usage constraints",
			suggestion: "delete the check: evo.Reason(name) never returns this error",
		},
		removedName{
			rule: "API-120", name: "ErrReasonWrongTask",
			message:    "evo.ErrReasonWrongTask was removed in 1.1: a Reason has no usage constraints",
			suggestion: "delete the check: evo.Reason(name) never returns this error",
		},
	)
}
