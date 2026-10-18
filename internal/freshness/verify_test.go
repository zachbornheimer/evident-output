package freshness

import (
	"context"
	"errors"
	"testing"
)

func TestEvaluateVerifiersAndsResultsAndReportsEachOutcome(t *testing.T) {
	yes := func(context.Context) (bool, error) { return true, nil }
	no := func(context.Context) (bool, error) { return false, nil }
	var told []VerifierOutcome
	satisfied, err := EvaluateVerifiers(t.Context(), []Verifier{yes, no, yes}, func(_ int, outcome VerifierOutcome) {
		told = append(told, outcome)
	})
	if err != nil || satisfied {
		t.Fatalf("satisfied=%v err=%v, want false with no error", satisfied, err)
	}
	want := []VerifierOutcome{VerifierSatisfied, VerifierUnsatisfied, VerifierSatisfied}
	if len(told) != len(want) {
		t.Fatalf("told %v, want %v", told, want)
	}
	for i := range want {
		if told[i] != want[i] {
			t.Fatalf("told %v, want %v", told, want)
		}
	}
}

// An observation error is distinct from a false result and takes priority:
// the run stops at it and later verifiers never run (§9.1).
func TestEvaluateVerifiersStopsAtTheFirstObservationError(t *testing.T) {
	boom := errors.New("cannot observe")
	ranAfter := false
	verifiers := []Verifier{
		func(context.Context) (bool, error) { return false, nil },
		func(context.Context) (bool, error) { return false, boom },
		func(context.Context) (bool, error) { ranAfter = true; return true, nil },
	}
	var outcomes []VerifierOutcome
	satisfied, err := EvaluateVerifiers(t.Context(), verifiers, func(_ int, outcome VerifierOutcome) {
		outcomes = append(outcomes, outcome)
	})
	if !errors.Is(err, boom) || satisfied {
		t.Fatalf("satisfied=%v err=%v, want the observation error", satisfied, err)
	}
	if ranAfter {
		t.Fatal("a verifier after the failing observation must not run")
	}
	if len(outcomes) != 2 || outcomes[1] != VerifierErrored {
		t.Fatalf("outcomes = %v, want the second reported as errored", outcomes)
	}
}

func TestEvaluateVerifiersOfNoneAreSatisfied(t *testing.T) {
	satisfied, err := EvaluateVerifiers(t.Context(), nil, nil)
	if err != nil || !satisfied {
		t.Fatalf("satisfied=%v err=%v, want vacuously satisfied", satisfied, err)
	}
}
