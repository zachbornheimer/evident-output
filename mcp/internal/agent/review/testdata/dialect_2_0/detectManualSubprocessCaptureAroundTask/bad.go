package fixture

import (
	"bytes"
	"io"
	"os/exec"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// runCaptured rebuilds Exec's capture with a buffer and a MultiWriter.
func runCaptured(task *evo.Task, cmd *exec.Cmd) (string, error) {
	var buf bytes.Buffer
	cmd.Stdout = io.MultiWriter(task.Writer(), &buf)
	cmd.Stderr = io.MultiWriter(task.Writer(), &buf)
	err := cmd.Run()
	return buf.String(), err
}
