package main

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"syscall"
)

const (
	evoModulePath   = "github.com/zachbornheimer/evident-output"
	mcpInstallPkg   = evoModulePath + "/cmd/evident-output-mcp"
	mcpLocalPkg     = "./cmd/evident-output-mcp"
	mcpBinaryName   = "evident-output-mcp"
	envNoAutoUpdate = "EVO_MCP_NO_AUTO_UPDATE"
	envBuiltFrom    = "EVO_MCP_BUILT_FROM"
	localBinRelHome = ".local/bin"
)

// Env reads process environment. Tests inject maps over t.TempDir paths.
type Env interface {
	Get(key string) string
}

// Files reads go.mod and creates the link directory.
type Files interface {
	ReadFile(name string) ([]byte, error)
	Stat(name string) (fs.FileInfo, error)
	MkdirAll(path string, perm os.FileMode) error
}

// Command is one exec the update runner records.
type Command struct {
	Name string
	Args []string
	Dir  string
	Env  []string
}

// Runner executes install steps. Tests record argv, cwd, and env.
type Runner interface {
	Run(c Command) error
}

// Execer replaces the current process (spawn-time re-exec). Tests record
// and never exec the test binary.
type Execer interface {
	Exec(argv0 string, argv, env []string) error
}

type osEnv struct{}

func (osEnv) Get(key string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	if key == "HOME" {
		h, _ := os.UserHomeDir()
		return h
	}
	return ""
}

type osFiles struct{}

func (osFiles) ReadFile(name string) ([]byte, error)  { return os.ReadFile(name) }
func (osFiles) Stat(name string) (fs.FileInfo, error) { return os.Stat(name) }
func (osFiles) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

type execRunner struct{}

func (execRunner) Run(c Command) error {
	cmd := exec.Command(c.Name, c.Args...)
	cmd.Dir = c.Dir
	cmd.Env = mergeEnv(os.Environ(), c.Env)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %v: %w", c.Name, c.Args, err)
	}
	return nil
}

type syscallExecer struct{}

func (syscallExecer) Exec(argv0 string, argv, env []string) error {
	return syscall.Exec(argv0, argv, env)
}

func mergeEnv(base, extra []string) []string {
	out := append([]string{}, base...)
	return append(out, extra...)
}
