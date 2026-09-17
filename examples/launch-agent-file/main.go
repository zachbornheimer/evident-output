// Command launch-agent-file demos spec §12's canonical launchd shape: a
// Sequence writing a managed plist via evo.File, followed by two
// Verify-gated Tasks stubbed to succeed. Run it twice against the same
// --state-dir to see the second run report "already satisfied" for
// register/start and perform no mutation for the plist File:
//
//	go run ./examples/launch-agent-file
//	go run ./examples/launch-agent-file   # second run: no mutation, already satisfied
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	evo "github.com/zachbornheimer/evident-output"
)

// agent is the domain data the real launchd integration would derive from
// caller configuration — hardcoded here since this example's point is the
// evo.File/Verify shape, not agent discovery.
type agent struct {
	label     string
	plistPath string
	plist     []byte
	mode      os.FileMode
}

func main() {
	stateDir := flag.String("state-dir", filepath.Join(os.TempDir(), "evo-launch-agent-file-example"), "manifest state directory (shared across runs to demo freshness)")
	flag.Parse()

	if err := os.MkdirAll(*stateDir, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "create state dir:", err)
		os.Exit(1)
	}
	a := agent{
		label:     "com.example.agent",
		plistPath: filepath.Join(*stateDir, "com.example.agent.plist"),
		plist:     []byte("<?xml version=\"1.0\"?><plist><dict><key>Label</key><string>com.example.agent</string></dict></plist>\n"),
		mode:      0o644,
	}
	registeredMarker := filepath.Join(*stateDir, "registered.marker")
	runningMarker := filepath.Join(*stateDir, "running.marker")

	evo.Init(evo.Config{Title: "launch agent", StateDir: *stateDir})
	os.Exit(evo.Main(func(ctx context.Context) error {
		return launchAgent(ctx, a, registeredMarker, runningMarker)
	}))
}

// launchAgent mirrors spec §12 exactly: evo.File needs no explicit Evidence
// declaration (it tracks itself through the manifest); register/start use
// the advanced read-only Verify for a non-file current-state check.
func launchAgent(ctx context.Context, a agent, registeredMarker, runningMarker string) error {
	seq := evo.Sequence("launch agent")

	write := seq.Task("write plist")
	write.Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{
			Path:     a.plistPath,
			Contents: a.plist,
			Mode:     a.mode,
		})
	})

	register := seq.Task("register")
	register.Verify(func(ctx context.Context) (bool, error) {
		return markerExists(registeredMarker)
	})
	register.Define(func(ctx context.Context) error {
		return writeMarker(registeredMarker)
	})

	start := seq.Task("start")
	start.Verify(func(ctx context.Context) (bool, error) {
		return markerExists(runningMarker)
	})
	start.Define(func(ctx context.Context) error {
		return writeMarker(runningMarker)
	})

	return nil
}

func markerExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func writeMarker(path string) error {
	return os.WriteFile(path, []byte("ok\n"), 0o644)
}
