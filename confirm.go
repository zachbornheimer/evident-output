package evo

import (
	"io"

	"github.com/zachbornheimer/evident-output/v2/internal/graph"
	txt "github.com/zachbornheimer/evident-output/v2/internal/project"
	"github.com/zachbornheimer/evident-output/v2/internal/project/live"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
	"github.com/zachbornheimer/evident-output/v2/internal/terminal"
)

// confirmPolicyHint is the Next step a Confirm nobody answered points at when the caller named no flag.
const confirmPolicyHint = "pass --yes to confirm non-interactively"

// ConfirmOption shapes a Confirm gate. It is sealed: only this package's constructors, and Detail, make one.
type ConfirmOption interface {
	applyConfirm(*confirmConfig)
}

type confirmOption func(*confirmConfig)

func (f confirmOption) applyConfirm(c *confirmConfig) { f(c) }

type confirmConfig struct {
	destructive bool
	// policyFlag is the caller's own flag that confirms non-interactively,
	// resolved when the hint is shown since only the run knows its title.
	policyFlag string
	// detail holds each Detail's context line, shown under the prompt.
	detail []string
}

// policyHint is the Next step an unasked refusal points at: the caller's flag, or the default "pass --yes".
func (c confirmConfig) policyHint(o *Run) Action {
	if c.policyFlag == "" {
		return label(confirmPolicyHint)
	}
	return actionCommand(o.selfName(), c.policyFlag)
}

// Destructive annotates the rendered prompt line as "(destructive)".
func Destructive() ConfirmOption {
	return confirmOption(func(c *confirmConfig) { c.destructive = true })
}

// NonInteractive names the flag of the caller's own executable that confirms without a prompt
// (for example "--yes"), shown in the hint a refusal nobody answered gives. An empty flag keeps the default hint.
func NonInteractive(flag string) ConfirmOption {
	return confirmOption(func(c *confirmConfig) { c.policyFlag = flag })
}

// Confirm quiesces the live region, renders a durable "?  <question>  [y/N]" gate, and reads one answer line,
// so no call site hand-rolls a prompt that fights the spinner or misreports "no" as failure. question is verbatim.
//
// "y" or "yes" (any case) succeeds the gate and returns true. Anything else refuses it ("declined"); a plain run
// refuses without asking ("no terminal to answer") when stdin is not a terminal, whatever the Output, and
// closed input refuses with "no answer — stdin closed"; both point at how to confirm non-interactively. Declining exits 1 through the Conclusion, never Failed or Cancelled.
// SIGINT or SIGTERM while waiting cancels the gate, and Confirm returns false.
func (r *Run) Confirm(question string, opts ...ConfirmOption) bool {
	if r == nil {
		return false
	}
	var cfg confirmConfig
	for _, opt := range opts {
		if opt != nil {
			opt.applyConfirm(&cfg)
		}
	}
	gate := r.confirmGate(question)
	r.mu.Lock()
	r.show.Stream.Ask(record.TaskID(gate.id))
	r.mu.Unlock()
	if !r.personCanAnswer() {
		return r.settleGate(gate, graph.AnswerNoTerminal, cfg)
	}
	return r.promptConfirm(gate, question, cfg)
}

// confirmGate is the Task a Confirm settles: declared like any root Task from
// the run body or a builder, and from inside a Define callback the library's
// own row, since asking there is work, not a declaration (CONTRACT §5).
func (r *Run) confirmGate(question string) *Task {
	if !r.graph.DeclaredInCallback() {
		return r.Task(question)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.taskHandle(r.graph.DeclareConfirmGate(question))
}

// settleGate resolves the gate as answer decides (graph.Answer.Outcome), commits its row, and reports whether it was yes.
func (r *Run) settleGate(gate *Task, answer graph.Answer, cfg confirmConfig) bool {
	switch answer.Outcome() {
	case cancelled:
		// The interrupt already resolved the gate.
		return false
	case succeeded:
		gate.succeed(answer.Summary())
	default:
		var next []ProblemOption
		if answer.Unasked() {
			next = append(next, Next(cfg.policyHint(r)))
		}
		gate.block(answer.Summary(), next...)
	}
	r.mu.Lock()
	r.show.Stream.Commit(record.TaskID(gate.id))
	r.mu.Unlock()
	return answer.Outcome() == succeeded
}

// promptConfirm asks question inside one Suspend window, so no live frame lands between the prompt and the
// resolved row. The abort channel is registered before anything is written, so a SIGINT from gate creation on
// unblocks the read.
func (r *Run) promptConfirm(gate *Task, question string, cfg confirmConfig) bool {
	abort, release := r.graph.RegisterAbort(gate.id)
	defer release()
	endTurn, ok := r.confirmTurn.Take(abort)
	if !ok {
		return r.settleGate(gate, graph.AnswerInterrupted, cfg)
	}
	defer endTurn()
	r.ownWindow(gate.id, true)
	defer r.ownWindow(gate.id, false)

	var yes bool
	_ = r.Suspend(func() error {
		r.mu.Lock()
		r.show.Stream.WriteOwner(txt.ConfirmPrompt(question, cfg.destructive, cfg.detail, r.humanStyle()))
		r.mu.Unlock()
		yes = r.settleGate(gate, graph.ReplyAnswer(terminal.ReadReply(r.confirmReader(), abort)), cfg)
		return nil
	})
	return yes
}

// ownWindow marks the gate id as the Task its window shows, so its rows are not held back like a sibling's, or clears the mark.
func (r *Run) ownWindow(id string, owns bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.show.Stream.OwnWindow(record.TaskID(id), owns)
}

// personCanAnswer reports whether a person can type an answer: stdin is a terminal whatever the Output, or
// the caller's Terminal facade is an interactive surface, whose keyboard is the Stdin facade (CONTRACT §5).
func (r *Run) personCanAnswer() bool {
	if terminal.ReaderIsTerminal(r.confirmReader()) {
		return true
	}
	surface := live.SurfaceOf(r.cfg.terminal)
	return surface != nil && surface.IsInteractive()
}

// confirmReader is the Stdin facade: standard input unless configured.
func (r *Run) confirmReader() io.Reader {
	if r.cfg.stdin != nil {
		return r.cfg.stdin
	}
	return terminal.Stdin()
}

// Suspend opens an exclusive-TTY window (Confirm's prompt, an interactive child program) around fn. It clears the
// live region and keeps every paint off the surface until fn returns; it does not hold the scheduler, so
// eligible siblings keep running (contract §3) and paint once it closes.
func (r *Run) Suspend(fn func() error) error {
	if fn == nil {
		return nil
	}
	r.mu.Lock()
	window := r.show.Live.Open()
	r.mu.Unlock()

	err := fn()

	r.mu.Lock()
	r.show.Live.Close(window, r.show.Stream.Emit)
	r.mu.Unlock()
	return err
}
