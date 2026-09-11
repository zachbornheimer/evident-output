package evo

import (
	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/engine"
)

// Action is a recommended next step for the user.
//
// Aliased into internal/core alongside the rest of the data model — see
// Snapshot's doc comment (snapshot.go) for why.
type Action = core.Action

// CommandSpec is an executable plus argv (never a shell string).
type CommandSpec = core.CommandSpec

// Command builds an action with an executable and arguments.
// Display-bound strings are sanitized at construction.
func Command(executable string, args ...string) Action {
	return engine.Command(executable, args...)
}

// Label builds a plain-text recommended next step with no executable command
// (e.g. a policy hint like "pass --yes to confirm non-interactively").
func Label(text string) Action { return engine.Label(text) }
