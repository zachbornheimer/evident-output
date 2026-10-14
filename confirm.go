package evo

import (
	"io"

	"github.com/zachbornheimer/evident-output/internal/graph"
	txt "github.com/zachbornheimer/evident-output/internal/project"
	"github.com/zachbornheimer/evident-output/internal/record"
	"github.com/zachbornheimer/evident-output/internal/terminal"
)

// confirmPolicyHint is the Next step a Confirm nobody answered points at,
// when the caller named no flag of its own.
const confirmPolicyHint = "pass --yes to confirm non-interactively"

// ConfirmOption shapes a Confirm gate. It is sealed: only this package's
// constructors make one, and Detail is one too.
type ConfirmOption interface {
	applyConfirm(*confirmConfig)
}

// confirmOption is every ConfirmOption but Detail.
type confirmOption func(*confirmConfig)

func (f confirmOption) applyConfirm(c *confirmConfig) { f(c) }

type confirmConfig struct {
	destructive bool
	// policyFlag is the flag of the caller's own executable that confirms
	// non-interactively, resolved against the run's identity when the hint
	// is shown, since only the run knows its title.
	policyFlag string
	// detail holds each Detail's context line, shown under the prompt.
	detail []string
}

// policyHint is the Next step an unasked refusal points at: the caller's
// flag on its own executable, or the default "pass --yes".
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

// NonInteractive names the flag of the caller's own executable that
// confirms without a prompt (default "--yes"), for the hint a refusal
// nobody answered shows.
func NonInteractive(argv ...string) ConfirmOption {
	flag := "--yes"
	if len(argv) > 0 {
		flag = argv[0]
	}
	return confirmOption(func(c *confirmConfig) { c.policyFlag = flag })
}

// Confirm quiesces the live region, renders a durable "?  <question>  [y/N]"
// gate, and reads one answer line — owning the whole gate so no call site
// hand-rolls a [y/N] prompt that fights the spinner or misreports "no" as
// failure (evo-rec.md "confirm gate" default). question is verbatim text,
// not a format.
//
// "y" or "yes" (any case) succeeds the gate and returns true. Anything else
// refuses it ("declined"); a plain run refuses without asking ("blocked by
// policy"), and input that closes before an answer refuses with "no answer —
// stdin closed"; both point at how to confirm non-interactively. Declining
// is never Failed or Cancelled: it exits 1 through the Conclusion. SIGINT or
// SIGTERM while waiting cancels the gate, and Confirm returns false.
func (o *Run) Confirm(question string, opts ...ConfirmOption) bool {
	if o == nil {
		return false
	}
	var cfg confirmConfig
	for _, opt := range opts {
		if opt != nil {
			opt.applyConfirm(&cfg)
		}
	}
	gate := o.Task(question)
	if o.cfg.plain {
		return o.settleGate(gate, graph.AnswerPolicy, cfg)
	}
	return o.promptConfirm(gate, question, cfg)
}

// settleGate resolves the gate as answer decides (graph.Answer.Outcome),
// commits its row at once, and reports whether the answer was yes.
func (o *Run) settleGate(gate *Task, answer graph.Answer, cfg confirmConfig) bool {
	switch answer.Outcome() {
	case cancelled:
		// The interrupt already resolved the gate.
		return false
	case succeeded:
		gate.succeed(answer.Summary())
	default:
		var next []ProblemOption
		if answer.Unasked() {
			next = append(next, Next(cfg.policyHint(o)))
		}
		gate.block(answer.Summary(), next...)
	}
	o.mu.Lock()
	o.show.Stream.Commit(record.TaskID(gate.id))
	o.mu.Unlock()
	return answer.Outcome() == succeeded
}

// promptConfirm asks question inside one Suspend window, so no live frame
// lands between the prompt and the gate's resolved row; siblings keep
// running and paint once it closes. The abort channel is registered before
// anything is written: a SIGINT from gate creation on unblocks the read
// instead of being swallowed (X2).
func (o *Run) promptConfirm(gate *Task, question string, cfg confirmConfig) bool {
	abort, release := o.graph.RegisterAbort(gate.id)
	defer release()
	endTurn, ok := o.confirmTurn.Take(abort)
	if !ok {
		return o.settleGate(gate, graph.AnswerInterrupted, cfg)
	}
	defer endTurn()
	o.ownWindow(gate.id, true)
	defer o.ownWindow(gate.id, false)

	var yes bool
	_ = o.Suspend(func() error {
		o.mu.Lock()
		o.show.Stream.WriteOwner(txt.ConfirmPrompt(question, cfg.destructive, cfg.detail, o.humanStyle()))
		o.mu.Unlock()
		yes = o.settleGate(gate, graph.ReplyAnswer(terminal.ReadReply(o.confirmReader(), abort)), cfg)
		return nil
	})
	return yes
}

// ownWindow marks the gate id as the Task its window shows, so its rows
// are not held back like a sibling's, or clears the mark.
func (o *Run) ownWindow(id string, owns bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.show.Stream.OwnWindow(record.TaskID(id), owns)
}

// confirmReader is the Stdin facade, the process's standard input unless
// configured.
func (o *Run) confirmReader() io.Reader {
	if o.cfg.stdin != nil {
		return o.cfg.stdin
	}
	return terminal.Stdin()
}

// Suspend opens an exclusive-TTY window (Confirm's prompt and answer, an
// interactive child program) around fn. It clears the live region and keeps
// every paint off the surface until fn returns, so a sibling's frame cannot
// overwrite the window; it does not hold the scheduler, so eligible
// siblings keep running (contract §3) and paint once the window closes.
func (o *Run) Suspend(fn func() error) error {
	if fn == nil {
		return nil
	}
	o.mu.Lock()
	window := o.show.Live.Open()
	o.mu.Unlock()

	err := fn()

	o.mu.Lock()
	o.show.Live.Close(window, o.show.Stream.Emit)
	o.mu.Unlock()
	return err
}
