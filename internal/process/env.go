package process

import (
	"os"
	"os/exec"
	"strings"
)

// Env returns the value of the named environment variable of this process,
// or "" when it is unset.
func Env(name string) string { return os.Getenv(name) }

// Environ returns this process's environment as "name=value" entries.
func Environ() []string { return os.Environ() }

// Args returns this process's command line, program name first.
func Args() []string { return os.Args }

// NewCmd returns a Cmd that will run the named program with args. Nothing
// starts until the caller runs it.
func NewCmd(name string, args ...string) *Cmd { return exec.Command(name, args...) }

// LookPath resolves a bare executable name on PATH.
func LookPath(name string) (string, error) { return exec.LookPath(name) }

// IsExplicitPath reports whether executable names a path (it contains a path
// separator) rather than a bare name to search on PATH.
func IsExplicitPath(executable string) bool {
	return strings.ContainsRune(executable, os.PathSeparator)
}
