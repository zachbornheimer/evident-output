package evo

import (
	"path/filepath"

	"github.com/zachbornheimer/evident-output/v2/internal/process"
)

// processArgv0 is the facade over the program name; a var so tests can inject it.
var processArgv0 = func() string {
	args := process.Args()
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

// selfName is the resolved Config.Name: the binary's basename when none was given.
func (r *Run) selfName() string {
	return r.cfg.subject
}

// identityFallbackName is the executable's basename or "command"; never a Subject.
func identityFallbackName() string {
	if base := filepath.Base(processArgv0()); base != "." && base != string(filepath.Separator) {
		return base
	}
	return "command"
}
