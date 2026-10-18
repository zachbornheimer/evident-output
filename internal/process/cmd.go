package process

import (
	"io"
	"os/exec"
)

// Cmd is a not-yet-run external command a Task runs directly. It exposes the
// command line, working directory, environment and output wiring a caller
// sets, and runs it; the operating-system process behind it stays private.
type Cmd struct {
	cmd *exec.Cmd
}

// NewCmd returns a Cmd that will run the named program with args. Nothing
// starts until the caller runs it.
func NewCmd(name string, args ...string) *Cmd {
	return &Cmd{cmd: exec.Command(name, args...)}
}

// Path is the program to run: the resolved path when the name was found on
// PATH, else the name as given.
func (c *Cmd) Path() string { return c.cmd.Path }

// Args is the command line, program name first.
func (c *Cmd) Args() []string { return c.cmd.Args }

// SetDir sets the working directory the command runs in; "" is the caller's.
func (c *Cmd) SetDir(dir string) { c.cmd.Dir = dir }

// SetEnv sets the command's environment as "name=value" entries.
func (c *Cmd) SetEnv(env []string) { c.cmd.Env = env }

// Stdout is where the command's standard output goes; nil discards it.
func (c *Cmd) Stdout() io.Writer { return c.cmd.Stdout }

// SetStdout sets where the command's standard output goes.
func (c *Cmd) SetStdout(w io.Writer) { c.cmd.Stdout = w }

// Stderr is where the command's standard error goes; nil discards it.
func (c *Cmd) Stderr() io.Writer { return c.cmd.Stderr }

// SetStderr sets where the command's standard error goes.
func (c *Cmd) SetStderr(w io.Writer) { c.cmd.Stderr = w }

// Run starts the command and waits for it to finish. A nonzero exit is
// returned as an error.
func (c *Cmd) Run() error { return c.cmd.Run() }

// CombinedOutput runs the command and returns its standard output and
// standard error together.
func (c *Cmd) CombinedOutput() ([]byte, error) { return c.cmd.CombinedOutput() }
