//go:build !unix

package engine

import "os/exec"

// isolateProcessGroup is a no-op where process groups are unavailable:
// cancellation kills the direct child only.
func isolateProcessGroup(*exec.Cmd) {}
