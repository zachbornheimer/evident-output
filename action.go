package evo

import "github.com/zachbornheimer/evident-output/internal/sanitize"

func sanitizeDisplay(s string) string { return sanitize.Text(s) }

// Action is a recommended next step for the user.
type Action struct {
	Label                string
	Command              *CommandSpec
	URL                  string
	File                 string
	Explanation          string
	RequiresConfirmation bool
	Destructive          bool
}

// CommandSpec is an executable plus argv (never a shell string).
type CommandSpec struct {
	Executable string
	Args       []string
	WorkingDir string
}

// Command builds an action with an executable and arguments.
// Display-bound strings are sanitized at construction.
func Command(executable string, args ...string) Action {
	copied := make([]string, len(args))
	for i, a := range args {
		copied[i] = sanitizeDisplay(a)
	}
	return Action{
		Command: &CommandSpec{
			Executable: sanitizeDisplay(executable),
			Args:       copied,
		},
	}
}

// Label builds a plain-text recommended next step with no executable command
// (e.g. a policy hint like "pass --yes to confirm non-interactively").
func Label(text string) Action {
	return Action{Label: sanitizeDisplay(text)}
}

func cloneActions(in []Action) []Action {
	if len(in) == 0 {
		return nil
	}
	out := make([]Action, len(in))
	copy(out, in)
	for i := range out {
		if out[i].Command != nil {
			cmd := *out[i].Command
			cmd.Args = append([]string(nil), cmd.Args...)
			out[i].Command = &cmd
		}
	}
	return out
}

func actionKey(a Action) string {
	if a.Command != nil {
		return "cmd:" + a.Command.Executable + " " + joinArgs(a.Command.Args)
	}
	if a.URL != "" {
		return "url:" + a.URL
	}
	if a.File != "" {
		return "file:" + a.File
	}
	return "label:" + a.Label + "|" + a.Explanation
}

func joinArgs(args []string) string {
	if len(args) == 0 {
		return ""
	}
	n := 0
	for _, a := range args {
		n += len(a) + 1
	}
	b := make([]byte, 0, n)
	for i, a := range args {
		if i > 0 {
			b = append(b, ' ')
		}
		b = append(b, a...)
	}
	return string(b)
}
