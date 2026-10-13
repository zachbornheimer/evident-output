package evo

import (
	"fmt"
)

// ExampleAction shows the recommended-next-step shape attached to a
// Problem or a resolved Task via Next/NextCommand.
func ExampleAction() {
	action := Action{Label: "retry with --force"}
	fmt.Println(action.Label)
	// Output:
	// retry with --force
}

// ExampleCommandSpec shows an executable plus argv — never a shell string —
// the shape Command builds and Action.Command carries.
func ExampleCommandSpec() {
	spec := commandSpec{Executable: "git", Args: []string{"push", "--force-with-lease"}}
	fmt.Println(spec.Executable, spec.Args)
	// Output:
	// git [push --force-with-lease]
}

// ExampleCommand builds a recommended next step naming an executable
// command to run.
func ExampleCommand() {
	action := actionCommand("git", "push", "--force-with-lease")
	fmt.Println(action.Command[0], action.Command[1:])
	// Output:
	// git [push --force-with-lease]
}

// ExampleLabel builds a plain-text recommended next step with no
// executable command — a policy hint like "pass --yes to confirm
// non-interactively".
func ExampleLabel() {
	action := label("pass --yes to confirm non-interactively")
	fmt.Println(action.Label)
	// Output:
	// pass --yes to confirm non-interactively
}
