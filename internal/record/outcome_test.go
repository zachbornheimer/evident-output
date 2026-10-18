package record

import "testing"

func TestRunOutcomeFollowsContractSectionFour(t *testing.T) {
	cases := []struct {
		name string
		in   []Outcome
		want Outcome
	}{
		{"no work succeeds", nil, OutcomeSucceeded},
		{"failed beats refused", []Outcome{OutcomeRefused, OutcomeFailed, OutcomeCancelled}, OutcomeFailed},
		{"refused beats cancelled", []Outcome{OutcomeCancelled, OutcomeRefused, OutcomeSucceeded}, OutcomeRefused},
		{"cancelled beats the rest", []Outcome{OutcomeSucceeded, OutcomeCancelled, OutcomeExcluded}, OutcomeCancelled},
		{"any ran work succeeds", []Outcome{OutcomeSatisfied, OutcomeSucceeded}, OutcomeSucceeded},
		{"all current is satisfied", []Outcome{OutcomeSatisfied, OutcomeSatisfied}, OutcomeSatisfied},
		{"excluded folds into satisfied", []Outcome{OutcomeExcluded, OutcomeSatisfied}, OutcomeSatisfied},
		{"excluded folds into succeeded", []Outcome{OutcomeExcluded, OutcomeSucceeded}, OutcomeSucceeded},
		{"everything excluded", []Outcome{OutcomeExcluded, OutcomeExcluded}, OutcomeExcluded},
		{"no outcome is ignored", []Outcome{"", OutcomeExcluded}, OutcomeExcluded},
	}
	for _, c := range cases {
		if got := RunOutcome(c.in...); got != c.want {
			t.Errorf("%s: RunOutcome(%v) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestOutcomeExitCodeFollowsContractSectionOne(t *testing.T) {
	want := map[Outcome]int{
		OutcomeSucceeded: 0, OutcomeSatisfied: 0, OutcomeExcluded: 0,
		OutcomeRefused: 1, OutcomeFailed: 2, OutcomeCancelled: 130,
	}
	for outcome, code := range want {
		if got := outcome.ExitCode(); got != code {
			t.Errorf("%s.ExitCode() = %d, want %d", outcome, got, code)
		}
	}
}
