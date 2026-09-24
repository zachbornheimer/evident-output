package main

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"

	evo "github.com/zachbornheimer/evident-output"
)

// plistMode is the launchd plist's managed permission bits.
const plistMode fs.FileMode = 0o644

// agent is the domain data a real launchd integration would derive from
// caller configuration — hardcoded here, since the point of this example
// is the one model serving both the CLI and HTTP projections.
type agent struct {
	plistPath string
	plist     []byte
	// load bootstraps the agent into launchd — the opaque mutation behind
	// the "load" Effect, injected so tests can stand in for launchctl.
	load func(context.Context) error
}

// newAgent is the example agent rooted in stateDir; loading it records a
// marker file where the real integration would run launchctl bootstrap.
func newAgent(stateDir string) agent {
	marker := filepath.Join(stateDir, "loaded.marker")
	return agent{
		plistPath: filepath.Join(stateDir, "com.example.agent.plist"),
		plist:     []byte("<?xml version=\"1.0\"?><plist><dict><key>Label</key><string>com.example.agent</string></dict></plist>\n"),
		load: func(context.Context) error {
			return os.WriteFile(marker, []byte("loaded\n"), plistMode)
		},
	}
}

// launchAgent declares the work once on whichever Output drives it: the
// package default for the CLI, a per-request Isolated Output for HTTP.
// Taking out as a parameter is what keeps concurrent requests apart —
// package-level evo.Task would land every request on one shared default.
func launchAgent(out *evo.Output, a agent) {
	seq := out.Sequence("launch agent")
	seq.Task("write plist").Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{Path: a.plistPath, Contents: a.plist, Mode: plistMode})
	})
	seq.Task("load agent").Define(func(ctx context.Context) error {
		return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectInstall, Object: "launch agent", Quantity: 1}, a.load)
	})
}
