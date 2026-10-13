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
