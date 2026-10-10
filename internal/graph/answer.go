package graph

import "github.com/zachbornheimer/evident-output/internal/record"

// Answer is what a Confirm gate received. The answer alone decides which
// state the gate Task reaches.
type Answer uint8

const (
	// AnswerYes is an affirmative reply typed at the prompt.
	AnswerYes Answer = iota
	// AnswerAssumedYes is a caller's flag answering for the human.
	AnswerAssumedYes
	// AnswerNo is any reply that is not affirmative, an empty line included.
	AnswerNo
	// AnswerPolicy is a refusal to ask: no terminal, plain output or a
	// non-interactive run, without a flag answering for the human.
	AnswerPolicy
	// AnswerClosed is stdin closing before any answer arrived. Nothing decided
	// to refuse; the stream gave no answer.
	AnswerClosed
	// AnswerInterrupted is an interrupt arriving while the question was open.
	// The interrupt itself settles the gate, so the answer settles nothing.
	AnswerInterrupted
)

// Outcome is the state the gate Task reaches for the answer: Done for yes,
// Blocked for every refusal (never Failed: declining is not an error), and
// Cancelled for an interrupt.
func (a Answer) Outcome() record.EntityState {
	switch a {
	case AnswerYes, AnswerAssumedYes:
		return record.Done
	case AnswerInterrupted:
		return record.Cancelled
	default:
		return record.Blocked
	}
}
