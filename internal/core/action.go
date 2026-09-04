package core

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
