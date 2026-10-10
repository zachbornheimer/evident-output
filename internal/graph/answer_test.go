package graph

import (
	"testing"

	"github.com/zachbornheimer/evident-output/internal/record"
)

func TestAnswerOutcomeDecidesTheGateState(t *testing.T) {
	cases := map[Answer]record.EntityState{
		AnswerYes:         record.Done,
		AnswerAssumedYes:  record.Done,
		AnswerNo:          record.Blocked,
		AnswerPolicy:      record.Blocked,
		AnswerClosed:      record.Blocked,
		AnswerInterrupted: record.Cancelled,
	}
	for answer, want := range cases {
		if got := answer.Outcome(); got != want {
			t.Errorf("Answer(%d).Outcome() = %s, want %s", answer, got, want)
		}
	}
}
