//go:build unix

package process

import (
	"os/exec"
	"syscall"
)

// isolateProcessGroup puts the child in its own process group and makes
// cancellation kill that whole group, so a grandchild cannot outlive a
// cancelled or timed-out Exec.
func isolateProcessGroup(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	c.Cancel = func() error {
		if c.Process == nil {
			return nil
		}
		return syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
	}
}
