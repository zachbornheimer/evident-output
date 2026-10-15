package evo

import (
	"slices"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

type (
	logRecord = record.LogRecord
)

// ProblemOption shapes the Problem that Fail, Refuse, Exclude or
// Task.Problem records. It is sealed: only this package's constructors
// make one.
type ProblemOption interface {
	recordOption() record.ProblemOption
}

// problemOption is every ProblemOption but Detail: one record option.
type problemOption struct{ opt record.ProblemOption }

func (o problemOption) recordOption() record.ProblemOption { return o.opt }

// detail is Detail's option: the longer explanation of a Problem, or a
// context line under a Confirm question.
type detail string

func (d detail) recordOption() record.ProblemOption { return record.WithDetail(string(d)) }

func (d detail) applyConfirm(c *confirmConfig) { c.detail = append(c.detail, string(d)) }

// applyProblemOptions is the Problem summary and opts describe; see
// record.ApplyProblemOptions. A nil option is skipped.
func applyProblemOptions(summary string, opts []ProblemOption) record.Problem {
	recordOpts := make([]record.ProblemOption, 0, len(opts))
	for _, opt := range opts {
		if opt != nil {
			recordOpts = append(recordOpts, opt.recordOption())
		}
	}
	return record.ApplyProblemOptions(summary, recordOpts)
}

// actionCommand builds an action with an executable and arguments.
func actionCommand(executable string, args ...string) Action {
	return asEvoAction(record.Command(executable, args...))
}

// label builds a plain-text recommended next step with no executable command.
func label(text string) Action { return asEvoAction(record.Label(text)) }

// Detail is the longer explanation of a Problem, or a context line under a
// Confirm question.
func Detail(text string) interface {
	ProblemOption
	ConfirmOption
} {
	return detail(text)
}

// Code sets a stable problem code.
func Code(value string) ProblemOption { return problemOption{record.WithCode(value)} }

// On sets the problem subject.
func On(subject string) ProblemOption { return problemOption{record.OnSubject(subject)} }

// Next attaches an action to a problem.
func Next(action Action) ProblemOption {
	return problemOption{record.WithAction(asRecordAction(action))}
}

// Action is a recommended next step: argv a machine can run, or text only a human can act on.
type Action struct {
	Command              []string
	Label                string
	uRL                  string
	file                 string
	explanation          string
	requiresConfirmation bool
	destructive          bool
}

func argvOf(cmd *commandSpec) []string {
	if cmd == nil {
		return nil
	}
	return append([]string{cmd.Executable}, cmd.Args...)
}

func commandOf(argv []string) *commandSpec {
	if len(argv) == 0 {
		return nil
	}
	return &commandSpec{Executable: argv[0], Args: slices.Clone(argv[1:])}
}

func asEvoAction(a record.Action) Action {
	return Action{
		Command: argvOf(a.Command), Label: a.Label, uRL: a.URL, file: a.File,
		explanation: a.Explanation, requiresConfirmation: a.RequiresConfirmation,
		destructive: a.Destructive,
	}
}

func asRecordAction(a Action) record.Action {
	return record.Action{
		Command: commandOf(a.Command), Label: a.Label, URL: a.uRL, File: a.file,
		Explanation: a.explanation, RequiresConfirmation: a.requiresConfirmation,
		Destructive: a.destructive,
	}
}

func asEvoActions(in []record.Action) []Action {
	if in == nil {
		return nil
	}
	out := make([]Action, len(in))
	for i, a := range in {
		out[i] = asEvoAction(a)
	}
	return out
}

func asRecordActions(in []Action) []record.Action {
	if in == nil {
		return nil
	}
	out := make([]record.Action, len(in))
	for i, a := range in {
		out[i] = asRecordAction(a)
	}
	return out
}
