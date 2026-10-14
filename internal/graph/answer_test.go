package graph

import (
	"testing"

	"github.com/zachbornheimer/evident-output/internal/record"
)

func TestAnswerOutcomeDecidesTheGateState(t *testing.T) {
	cases := map[Answer]record.State{
		AnswerYes:         record.Succeeded,
		AnswerAssumedYes:  record.Succeeded,
		AnswerNo:          record.Refused,
		AnswerPolicy:      record.Refused,
		AnswerClosed:      record.Refused,
		AnswerInterrupted: record.Cancelled,
	}
	for answer, want := range cases {
		if got := answer.Outcome(); got != want {
			t.Errorf("Answer(%d).Outcome() = %s, want %s", answer, got, want)
		}
	}
}

func TestAnswerSummaryAndHint(t *testing.T) {
	cases := []struct {
		answer  Answer
		summary string
		unasked bool
	}{
		{AnswerYes, "", false},
		{AnswerAssumedYes, "assumed --yes", false},
		{AnswerNo, "declined", false},
		{AnswerPolicy, "blocked by policy", true},
		{AnswerClosed, "no answer — stdin closed", true},
		{AnswerInterrupted, "", false},
	}
	for _, c := range cases {
		if got := c.answer.Summary(); got != c.summary {
			t.Errorf("Answer(%d).Summary() = %q, want %q", c.answer, got, c.summary)
		}
		if got := c.answer.Unasked(); got != c.unasked {
			t.Errorf("Answer(%d).Unasked() = %v, want %v", c.answer, got, c.unasked)
		}
	}
}
