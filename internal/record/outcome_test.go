package record

import "testing"

func TestWorstOutcomeRanksFailedRefusedCancelledThenTheRest(t *testing.T) {
	cases := []struct {
		name string
		in   []Outcome
		want Outcome
	}{
		{"none", nil, ""},
		{"failed beats refused", []Outcome{OutcomeRefused, OutcomeFailed, OutcomeCancelled}, OutcomeFailed},
		{"refused beats cancelled", []Outcome{OutcomeCancelled, OutcomeRefused, OutcomeSucceeded}, OutcomeRefused},
		{"cancelled beats the rest", []Outcome{OutcomeSucceeded, OutcomeCancelled, OutcomeExcluded}, OutcomeCancelled},
		{"first of the rest wins", []Outcome{OutcomeSatisfied, OutcomeSucceeded}, OutcomeSatisfied},
		{"single outcome", []Outcome{OutcomeExcluded}, OutcomeExcluded},
	}
	for _, c := range cases {
		if got := WorstOutcome(c.in...); got != c.want {
			t.Errorf("%s: WorstOutcome(%v) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}
