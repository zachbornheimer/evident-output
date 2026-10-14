package graph

import (
	"github.com/zachbornheimer/evident-output/v2/internal/record"
	input "github.com/zachbornheimer/evident-output/v2/internal/terminal"
)

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

// Outcome is the state the gate Task reaches for the answer: Succeeded for yes,
// Refused for every refusal (never Failed: declining is not an error), and
// Cancelled for an interrupt.
func (a Answer) Outcome() record.State {
	switch a {
	case AnswerYes, AnswerAssumedYes:
		return record.Succeeded
	case AnswerInterrupted:
		return record.Cancelled
	default:
		return record.Refused
	}
}

// Summary is the gate Task's row text for the answer: one spelling per
// refusal, so no caller composes wording a script might parse. Yes typed at
// the prompt and an interrupt need none.
func (a Answer) Summary() string {
	switch a {
	case AnswerAssumedYes:
		return "assumed --yes"
	case AnswerNo:
		return "declined"
	case AnswerPolicy:
		return "blocked by policy"
	case AnswerClosed:
		return "no answer — stdin closed"
	default:
		return ""
	}
}

// Unasked reports a refusal nobody made on purpose — a policy that would
// not ask, or input that closed — whose row points at how to confirm
// non-interactively.
func (a Answer) Unasked() bool { return a == AnswerPolicy || a == AnswerClosed }

// ReplyAnswer is the Answer a reply read at the prompt gives.
func ReplyAnswer(r input.Reply) Answer {
	switch r {
	case input.ReplyYes:
		return AnswerYes
	case input.ReplyClosed:
		return AnswerClosed
	case input.ReplyInterrupted:
		return AnswerInterrupted
	default:
		return AnswerNo
	}
}
